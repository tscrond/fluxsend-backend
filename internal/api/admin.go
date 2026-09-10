package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/config"
	chimiddleware "github.com/tscrond/fluxsend-backend/internal/middleware"
	"github.com/tscrond/fluxsend-backend/internal/service"
)

type AdminServer struct {
	*CoreHandlers
	routePrefix  string
	AdminService service.AdminService
}

type AdminServerDependencies struct {
	CoreHandlersDependencies
	AdminService service.AdminService
}

func NewAdminServer(backendConfig config.BackendConfig, deps AdminServerDependencies) *AdminServer {
	return &AdminServer{
		CoreHandlers: NewCoreHandlers(backendConfig, deps.CoreHandlersDependencies),
		routePrefix:  "/admin",
		AdminService: deps.AdminService,
	}
}

func (s *AdminServer) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestLogger(s.log))

	r.Route(s.routePrefix, func(r chi.Router) {
		s.registerAdminRoutes(r)
	})
	return r
}

func (s *AdminServer) registerAdminRoutes(r chi.Router) {
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	adminUserMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok || username != s.backendConfig.AdminUsername || password != s.backendConfig.AdminPassword {
				w.Header().Set("WWW-Authenticate", `Basic realm="admin"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	r.With(adminUserMiddleware).Get("/users", s.listUsersHandler)
	r.With(adminUserMiddleware).Get("/users/{user_id}", s.getUserHandler)
	r.With(adminUserMiddleware).Post("/users", s.createUserHandler)
	r.With(adminUserMiddleware).Delete("/users/{user_id}", s.deleteUserHandler)
	r.With(adminUserMiddleware).Post("/users/{user_id}/plan", s.assignPlanHandler)
	r.With(adminUserMiddleware).Get("/plans", s.listPlansHandler)
	r.With(adminUserMiddleware).Post("/plans", s.createPlanHandler)
	r.With(adminUserMiddleware).Put("/plans/{plan_id}", s.updatePlanHandler)
	r.With(adminUserMiddleware).Get("/capacity", s.capacityHandler)
}

func (s *AdminServer) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := s.AdminService.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}

func (s *AdminServer) getUserHandler(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	user, err := s.AdminService.GetUser(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *AdminServer) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	user, err := s.AdminService.CreateUser(r.Context(), payload.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *AdminServer) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	user, err := s.AdminService.DeleteUser(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *AdminServer) assignPlanHandler(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	var payload struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	planID, err := uuid.Parse(payload.PlanID)
	if err != nil {
		http.Error(w, "invalid plan id", http.StatusBadRequest)
		return
	}
	if err := s.AdminService.AssignUserPlan(r.Context(), userID, planID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *AdminServer) listPlansHandler(w http.ResponseWriter, r *http.Request) {
	plans, err := s.AdminService.ListPlans(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plans)
}

func (s *AdminServer) createPlanHandler(w http.ResponseWriter, r *http.Request) {
	var req service.CreatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	plan, err := s.AdminService.CreatePlan(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plan)
}

func (s *AdminServer) updatePlanHandler(w http.ResponseWriter, r *http.Request) {
	planID, err := uuid.Parse(chi.URLParam(r, "plan_id"))
	if err != nil {
		http.Error(w, "invalid plan id", http.StatusBadRequest)
		return
	}
	var req service.CreatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	if err := s.AdminService.UpdatePlan(r.Context(), planID, req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *AdminServer) capacityHandler(w http.ResponseWriter, r *http.Request) {
	capacity, err := s.AdminService.CapacitySummary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(capacity)
}
