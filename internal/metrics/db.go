package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"strings"
	"unicode"

	"github.com/lib/pq"
)

var sqlcQueryNamePattern = regexp.MustCompile(`(?m)^-- name: ([A-Za-z0-9_]+)\s`)

func OpenInstrumentedPostgres(dsn string, recorder *Metrics) (*sql.DB, error) {
	if recorder == nil || !recorder.Enabled() {
		return sql.Open("postgres", dsn)
	}

	connector := &instrumentedConnector{
		dsn:     dsn,
		driver:  pq.Driver{},
		metrics: recorder,
	}
	return sql.OpenDB(connector), nil
}

type instrumentedConnector struct {
	dsn     string
	driver  driver.Driver
	metrics *Metrics
}

func (c *instrumentedConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		c.metrics.observeDBError("connect", err)
		return nil, err
	}
	return &instrumentedConn{conn: conn, metrics: c.metrics}, nil
}

func (c *instrumentedConnector) Driver() driver.Driver {
	return c.driver
}

type instrumentedConn struct {
	conn    driver.Conn
	metrics *Metrics
}

func (c *instrumentedConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.conn.Prepare(query)
	c.metrics.observeDBError(operationLabelFromQuery(query), err)
	if err != nil {
		return nil, err
	}
	return wrapStmt(stmt, c.metrics, query), nil
}

func (c *instrumentedConn) Close() error {
	return c.conn.Close()
}

func (c *instrumentedConn) Begin() (driver.Tx, error) {
	tx, err := c.conn.Begin()
	c.metrics.observeDBError("begin_tx", err)
	return tx, err
}

func (c *instrumentedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if beginTx, ok := c.conn.(driver.ConnBeginTx); ok {
		tx, err := beginTx.BeginTx(ctx, opts)
		c.metrics.observeDBError("begin_tx", err)
		return tx, err
	}
	return c.Begin()
}

func (c *instrumentedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if preparer, ok := c.conn.(driver.ConnPrepareContext); ok {
		stmt, err := preparer.PrepareContext(ctx, query)
		c.metrics.observeDBError(operationLabelFromQuery(query), err)
		if err != nil {
			return nil, err
		}
		return wrapStmt(stmt, c.metrics, query), nil
	}
	return c.Prepare(query)
}

func (c *instrumentedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if execer, ok := c.conn.(driver.ExecerContext); ok {
		result, err := execer.ExecContext(ctx, query, args)
		c.metrics.observeDBError(operationLabelFromQuery(query), err)
		return result, err
	}
	return nil, driver.ErrSkip
}

func (c *instrumentedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if queryer, ok := c.conn.(driver.QueryerContext); ok {
		rows, err := queryer.QueryContext(ctx, query, args)
		c.metrics.observeDBError(operationLabelFromQuery(query), err)
		return rows, err
	}
	return nil, driver.ErrSkip
}

func (c *instrumentedConn) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := c.conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
	}
	return nil
}

func (c *instrumentedConn) Ping(ctx context.Context) error {
	if pinger, ok := c.conn.(driver.Pinger); ok {
		err := pinger.Ping(ctx)
		c.metrics.observeDBError("ping", err)
		return err
	}
	return nil
}

func (c *instrumentedConn) ResetSession(ctx context.Context) error {
	if resetter, ok := c.conn.(driver.SessionResetter); ok {
		return resetter.ResetSession(ctx)
	}
	return nil
}

func (c *instrumentedConn) IsValid() bool {
	if validator, ok := c.conn.(driver.Validator); ok {
		return validator.IsValid()
	}
	return true
}

type instrumentedStmt struct {
	stmt      driver.Stmt
	metrics   *Metrics
	operation string
}

func wrapStmt(stmt driver.Stmt, metrics *Metrics, query string) driver.Stmt {
	return &instrumentedStmt{stmt: stmt, metrics: metrics, operation: operationLabelFromQuery(query)}
}

func (s *instrumentedStmt) Close() error {
	return s.stmt.Close()
}

func (s *instrumentedStmt) NumInput() int {
	return s.stmt.NumInput()
}

func (s *instrumentedStmt) Exec(args []driver.Value) (driver.Result, error) {
	result, err := s.stmt.Exec(args)
	s.metrics.observeDBError(s.operation, err)
	return result, err
}

func (s *instrumentedStmt) Query(args []driver.Value) (driver.Rows, error) {
	rows, err := s.stmt.Query(args)
	s.metrics.observeDBError(s.operation, err)
	return rows, err
}

func (s *instrumentedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if execer, ok := s.stmt.(driver.StmtExecContext); ok {
		result, err := execer.ExecContext(ctx, args)
		s.metrics.observeDBError(s.operation, err)
		return result, err
	}
	return nil, driver.ErrSkip
}

func (s *instrumentedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if queryer, ok := s.stmt.(driver.StmtQueryContext); ok {
		rows, err := queryer.QueryContext(ctx, args)
		s.metrics.observeDBError(s.operation, err)
		return rows, err
	}
	return nil, driver.ErrSkip
}

func operationLabelFromQuery(query string) string {
	matches := sqlcQueryNamePattern.FindStringSubmatch(query)
	if len(matches) == 2 {
		return camelToSnake(matches[1])
	}

	for _, line := range strings.Split(query, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			break
		}
		return strings.ToLower(fields[0])
	}

	return "unknown"
}

func camelToSnake(value string) string {
	var builder strings.Builder
	for i, r := range value {
		if unicode.IsUpper(r) {
			if i > 0 {
				builder.WriteByte('_')
			}
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		builder.WriteRune(unicode.ToLower(r))
	}
	return builder.String()
}
