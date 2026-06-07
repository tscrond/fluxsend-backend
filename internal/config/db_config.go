package config

import "fmt"

type DBConfig struct {
	Host     string
	User     string
	Password string
	Name     string
}

func (cfg DBConfig) ConnString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", cfg.User, cfg.Password, cfg.Host, cfg.Name)
}
