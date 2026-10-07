package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"linfbbpages/internal/fbb"
)

const (
	maxJSONBody = 1 << 20
	maxPageSize = 200
)

type Server struct {
	store      *fbb.Store
	sessions   *sessions
	static     http.Handler
	logger     *log.Logger
	sessionTTL time.Duration
}

func NewServer(store *fbb.Store, sessionTTL time.Duration, static http.Handler, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{
		store:      store,
		sessions:   newSessions(sessionTTL),
		static:     static,
		logger:     logger,
		sessionTTL: sessionTTL,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
		serveAPI(s, w, r)
		return
	}
	if s.static == nil {
		http.NotFound(w, r)
		return
	}
	s.static.ServeHTTP(w, r)
}

func serveAPI(s *Server, w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/login":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.login(w, r)
	case r.URL.Path == "/api/logout":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.logout(w, r)
	case r.URL.Path == "/api/me":
		if !s.requireUser(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		s.me(w, r)
	case r.URL.Path == "/api/messages":
		if !s.requireUser(w, r) {
			return
		}
		s.messages(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/messages/"):
		if !s.requireUser(w, r) {
			return
		}
		s.message(w, r)
	case r.URL.Path == "/api/files":
		if !s.requireUser(w, r) {
			return
		}
		s.files(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/files/"):
		if !s.requireUser(w, r) {
			return
		}
		s.file(w, r)
	default:
		http.NotFound(w, r)
	}
}

type loginRequest struct {
	Callsign string `json:"callsign"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(request.Callsign) > 32 || len(request.Password) > 128 || strings.TrimSpace(request.Callsign) == "" {
		writeError(w, http.StatusBadRequest, "callsign o contraseña inválidos")
		return
	}
	user, authenticated, err := s.store.Authenticate(request.Callsign, request.Password)
	if err != nil {
		s.logger.Printf("login: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo leer la base de usuarios")
		return
	}
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "callsign o contraseña incorrectos")
		return
	}
	token, err := s.sessions.create(user)
	if err != nil {
		s.logger.Printf("session: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo crear la sesión")
		return
	}
	setSessionCookie(w, token, s.sessionTTL)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.delete(cookie.Value)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := currentUser(r); ok {
		return true
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		if user, ok := s.sessions.get(cookie.Value); ok {
			*r = *r.WithContext(withUser(r.Context(), user))
			return true
		}
	}
	writeError(w, http.StatusUnauthorized, "sesión requerida")
	return false
}

type userContextKey struct{}

func withUser(ctx context.Context, user fbb.User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func currentUser(r *http.Request) (fbb.User, bool) {
	user, ok := r.Context().Value(userContextKey{}).(fbb.User)
	return user, ok
}

func messageVisibleTo(message fbb.Message, user fbb.User) bool {
	if user.Sysop {
		return true
	}
	if message.Status == "$" || message.Status == "K" {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(message.Type)) {
	case "B":
		return true
	case "P", "A", "T":
		return sameCallsign(message.From, user.Callsign) || sameCallsign(message.To, user.Callsign)
	default:
		return false
	}
}

func sameCallsign(left, right string) bool {
	left = callsignWithoutSSID(left)
	right = callsignWithoutSSID(right)
	return left != "" && strings.EqualFold(left, right)
}

func callsignWithoutSSID(callsign string) string {
	callsign = strings.TrimSpace(callsign)
	if index := strings.IndexByte(callsign, '-'); index >= 0 {
		callsign = callsign[:index]
	}
	return callsign
}

func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listMessages(w, r)
	case http.MethodPost:
		s.composeMessage(w, r)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sesión requerida")
		return
	}
	all, err := s.store.Messages()
	if err != nil {
		s.logger.Printf("messages: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo leer el índice de mensajes")
		return
	}
	query := r.URL.Query()
	typeFilter := strings.ToUpper(strings.TrimSpace(query.Get("type")))
	textFilter := strings.ToLower(strings.TrimSpace(query.Get("q")))
	filtered := make([]fbb.Message, 0, len(all))
	for _, message := range all {
		if !messageVisibleTo(message, user) {
			continue
		}
		if typeFilter != "" && message.Type != typeFilter {
			continue
		}
		if textFilter != "" && !strings.Contains(strings.ToLower(message.Title), textFilter) && !strings.Contains(strings.ToLower(message.From), textFilter) && !strings.Contains(strings.ToLower(message.To), textFilter) {
			continue
		}
		filtered = append(filtered, message)
	}

	page, pageSize, err := pagination(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	items := filtered[start:end]
	if items == nil {
		items = []fbb.Message{}
	}
	totalPages := (len(filtered) + pageSize - 1) / pageSize
	writeJSON(w, http.StatusOK, map[string]any{
		"messages":    items,
		"page":        page,
		"page_size":   pageSize,
		"total":       len(filtered),
		"total_pages": totalPages,
	})
}

func pagination(query url.Values) (int, int, error) {
	page := 1
	pageSize := 50
	var err error
	if value := query.Get("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil || page < 1 {
			return 0, 0, errors.New("page inválida")
		}
	}
	if value := query.Get("page_size"); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil || pageSize < 1 || pageSize > maxPageSize {
			return 0, 0, fmt.Errorf("page_size debe estar entre 1 y %d", maxPageSize)
		}
	}
	return page, pageSize, nil
}

func (s *Server) message(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	number, ok := messageNumber(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "número de mensaje inválido")
		return
	}
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sesión requerida")
		return
	}
	indexedMessages, err := s.store.Messages()
	if err != nil {
		s.logger.Printf("message index: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo leer el índice de mensajes")
		return
	}
	var indexedMessage fbb.Message
	found := false
	for _, candidate := range indexedMessages {
		if candidate.Number == number {
			indexedMessage = candidate
			found = true
			break
		}
	}
	if !found || !messageVisibleTo(indexedMessage, user) {
		writeError(w, http.StatusNotFound, "mensaje no encontrado")
		return
	}
	message, body, err := s.store.Message(number)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "mensaje no encontrado")
			return
		}
		s.logger.Printf("message %d: %v", number, err)
		writeError(w, http.StatusInternalServerError, "no se pudo leer el mensaje")
		return
	}
	if !messageVisibleTo(message, user) {
		writeError(w, http.StatusNotFound, "mensaje no encontrado")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": message, "body": body})
}

func messageNumber(path string) (int64, bool) {
	part := strings.TrimPrefix(path, "/api/messages/")
	part, err := url.PathUnescape(part)
	if err != nil || part == "" || strings.Contains(part, "/") {
		return 0, false
	}
	number, err := strconv.ParseInt(part, 10, 64)
	return number, err == nil && number > 0
}

type composeRequest struct {
	To    string `json:"to"`
	Route string `json:"route"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (s *Server) composeMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sesión requerida")
		return
	}
	var request composeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := s.store.AppendMail(user.Callsign, fbb.ComposeRequest{
		To: request.To, Route: request.Route, Type: strings.ToUpper(request.Type), Title: request.Title, Body: request.Body,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"message": "Mensaje encolado; FBB lo procesará en aproximadamente un minuto.",
	})
}

func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	files, err := s.store.DecodedFiles()
	if err != nil {
		s.logger.Printf("files: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo leer la carpeta de archivos decodificados")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/files/")
	name, err := url.PathUnescape(name)
	if err != nil || name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		writeError(w, http.StatusBadRequest, "nombre de archivo inválido")
		return
	}
	file, info, err := s.store.ReadDecodedFile(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "archivo no encontrado")
			return
		}
		writeError(w, http.StatusBadRequest, "nombre de archivo inválido")
		return
	}
	defer file.Close()
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("JSON inválido: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("JSON inválido: se recibió más de un objeto")
		}
		return fmt.Errorf("JSON inválido: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "método no permitido")
}
