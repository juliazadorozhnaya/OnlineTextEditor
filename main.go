package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
)

// JWT handling

var jwtKey = []byte("my_secret_key") // Секретный ключ для подписи JWT

type Claims struct {
	Username string `json:"username"`
	jwt.StandardClaims
}

// Генерация JWT токена
func generateJWT(username string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: username,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// Проверка JWT токена
func validateJWT(tokenStr string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, err
	}

	return claims, nil
}

// WebSocket handling

type client struct {
	conn      *websocket.Conn
	send      chan []byte
	hub       *hub
	file      *file
	username  string
	sessionId string
}

func (c *client) read() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		c.hub.broadcast <- msg
	}
}

func (c *client) write() {
	defer c.conn.Close()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func serveWs(h *hub, f *file, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	sessionID := r.URL.Query().Get("session_id") // Предполагаем, что session_id передаётся в запросе
	if sessionID == "" {
		sessionID = uuid.NewString() // Генерируем новый, если не был передан
	}

	username := r.Context().Value("username").(string)
	client := &client{
		conn:      conn,
		send:      make(chan []byte, 256),
		hub:       h,
		file:      f,
		username:  username,
		sessionId: sessionID, // Используем sessionID
	}
	client.hub.register <- client

	// Отправка текущего текста и времени подключения новому клиенту
	text, err := f.read()
	if err == nil {
		client.send <- []byte(fmt.Sprintf(`{"type": "text", "text": %q, "session_id": %q}`, text, sessionID))
	}
	startTimeMessage := fmt.Sprintf(`{"type": "startTime", "startTime": %d}`, h.startTime.Unix())
	client.send <- []byte(startTimeMessage)
	fontSizeMessage := fmt.Sprintf(`{"type": "fontSize", "fontSize": %d}`, h.fontSize)
	client.send <- []byte(fontSizeMessage)

	go client.write()
	go client.read()
}

type file struct {
	name string
}

func newFile(filename string) *file {
	return &file{name: filename}
}

func (f *file) read() (string, error) {
	byte, err := ioutil.ReadFile(f.name)
	if err != nil {
		return "", err
	}

	return string(byte), nil
}

func (f *file) write(b []byte) error {
	err := ioutil.WriteFile(f.name, b, 0666)
	if err != nil {
		return err
	}
	return nil
}

// Hub handling

type hub struct {
	clients    map[*client]bool
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
	usernames  map[string]int
	file       *file
	startTime  time.Time
	fontSize   int
}

func newHub(f *file) *hub {
	return &hub{
		broadcast:  make(chan []byte),
		register:   make(chan *client),
		unregister: make(chan *client),
		clients:    make(map[*client]bool),
		usernames:  make(map[string]int),
		file:       f,
		startTime:  time.Now(), // Initialize start time
		fontSize:   16,         // Initialize font size
	}
}

func (h *hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.usernames[client.username]++
			h.updateUsers()
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.usernames[client.username]--
				if h.usernames[client.username] == 0 {
					delete(h.usernames, client.username)
				}
				h.updateUsers()
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
					h.usernames[client.username]--
					if h.usernames[client.username] == 0 {
						delete(h.usernames, client.username)
					}
					h.updateUsers()
				}
			}

			// Write the received message to the file
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				if msgType, ok := msg["type"].(string); ok {
					if msgType == "text" {
						if text, ok := msg["text"].(string); ok {
							h.file.write([]byte(text))
						}
					} else if msgType == "fontSize" {
						if fontSize, ok := msg["fontSize"].(float64); ok {
							h.fontSize = int(fontSize)
						}
					}
				}
			}
		}
	}
}

func (h *hub) updateUsers() {
	users := []string{}
	for username := range h.usernames {
		users = append(users, username)
	}
	data := map[string]interface{}{
		"type":  "users",
		"users": users,
	}
	message, _ := json.Marshal(data)
	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// HTTP server handling

var store = sessions.NewCookieStore([]byte("something-very-secret"))

const (
	ExitCodeOk    = 0
	ExitCodeError = 1
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitCodeError)
	}

	os.Exit(ExitCodeOk)
}

func run() error {
	fmt.Println("start server")

	file := newFile(filepath.Join("data", "text.txt"))
	h := newHub(file)
	go h.run()

	r := mux.NewRouter()
	r.HandleFunc("/", showHomePage).Methods("GET")
	r.HandleFunc("/login", handleLogin).Methods("POST")
	r.HandleFunc("/ws/{roomID}", func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session")
		token, ok := session.Values["token"].(string)
		if !ok || token == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		claims, err := validateJWT(token)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Add username to the request context
		r = r.WithContext(context.WithValue(r.Context(), "username", claims.Username))

		serveWs(h, file, w, r)
	})
	r.Handle("/editor/{roomID}", withAuth(&initHandler{file: file})).Methods("GET")

	if err := http.ListenAndServe(":8080", r); err != nil {
		return err
	}

	return nil
}

func showHomePage(w http.ResponseWriter, r *http.Request) {
	tpl := template.Must(
		template.ParseFiles(filepath.Join("templates", "index.html")),
	)

	tpl.Execute(w, nil)
}

// Установка куки
func setCookie(w http.ResponseWriter, name, value string, expiration time.Time) {
	cookie := http.Cookie{
		Name:    name,
		Value:   value,
		Expires: expiration,
		Path:    "/", // Устанавливаем путь, чтобы кука была доступна для всех маршрутов
	}
	http.SetCookie(w, &cookie)
	log.Printf("Cookie %s set to %s", name, value)
}

// Получение куки
func getCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		log.Printf("Cookie %s not found", name)
		return "", err
	}
	return cookie.Value, nil
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")

	// Проверка куки на существующего пользователя
	cookieName := "last_login"
	lastLogin, err := getCookie(r, cookieName)
	if err == nil {
		// Кука существует, проверим, не истекла ли она
		lastLoginTime, err := time.Parse(time.RFC3339, lastLogin)
		if err == nil && time.Since(lastLoginTime) < 24*time.Hour {
			// Пользователь уже регистрировался сегодня, перенаправляем в редактор
			log.Printf("User %s already logged in today", username)
			token, err := generateJWT(username)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			session, _ := store.Get(r, "session")
			session.Values["token"] = token
			session.Save(r, w)

			// Generate a unique room ID
			roomID := uuid.New().String()

			http.Redirect(w, r, fmt.Sprintf("/editor/%s", roomID), http.StatusFound)
			return
		}
	}

	// Генерация JWT токена
	token, err := generateJWT(username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["token"] = token
	session.Save(r, w)

	// Устанавливаем куку с текущим временем
	expiration := time.Now().Add(24 * time.Hour)
	setCookie(w, cookieName, time.Now().Format(time.RFC3339), expiration)

	// Generate a unique room ID
	roomID := uuid.New().String()

	http.Redirect(w, r, fmt.Sprintf("/editor/%s", roomID), http.StatusFound)
}

// Middleware для проверки авторизации
func withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session")
		token, ok := session.Values["token"].(string)
		if !ok || token == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		claims, err := validateJWT(token)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Add username to the request context
		ctx := context.WithValue(r.Context(), "username", claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Обработчик для начальной загрузки страницы редактора
type initHandler struct {
	file *file
}

func (i *initHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tpl := template.Must(
		template.ParseFiles(filepath.Join("templates", "editor.html")),
	)

	text, err := i.file.read()
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get username from the request context
	username := r.Context().Value("username").(string)
	roomID := mux.Vars(r)["roomID"]

	m := map[string]interface{}{
		"Text":     text,
		"Host":     fmt.Sprintf("%s/ws/%s", r.Host, roomID),
		"Username": username,
		"RoomID":   roomID,
	}

	tpl.Execute(w, m)
}
