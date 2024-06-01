package handler

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/juliazadorozhnaya/OnlineTextEditor/auth"
	"github.com/juliazadorozhnaya/OnlineTextEditor/file"
	"log"
	"net/http"
	"path/filepath"
	"text/template"
	"time"
)

var store = sessions.NewCookieStore([]byte("something-very-secret"))

func ShowHomePage(w http.ResponseWriter, r *http.Request) {
	tpl := template.Must(
		template.ParseFiles(filepath.Join("static", "index.html")),
	)

	tpl.Execute(w, nil)
}

// Установка куки
func SetCookie(w http.ResponseWriter, name, value string, expiration time.Time) {
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
func GetCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		log.Printf("Cookie %s not found", name)
		return "", err
	}
	return cookie.Value, nil
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")

	// Проверка куки на существующего пользователя
	cookieName := "last_login"
	lastLogin, err := GetCookie(r, cookieName)
	if err == nil {
		// Кука существует, проверим, не истекла ли она
		lastLoginTime, err := time.Parse(time.RFC3339, lastLogin)
		if err == nil && time.Since(lastLoginTime) < 24*time.Hour {
			// Пользователь уже регистрировался сегодня, перенаправляем в редактор
			log.Printf("User %s already logged in today", username)
			token, err := auth.GenerateJWT(username)
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
	token, err := auth.GenerateJWT(username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["token"] = token
	session.Save(r, w)

	// Устанавливаем куку с текущим временем
	expiration := time.Now().Add(24 * time.Hour)
	SetCookie(w, cookieName, time.Now().Format(time.RFC3339), expiration)

	// Generate a unique room ID
	roomID := uuid.New().String()

	http.Redirect(w, r, fmt.Sprintf("/editor/%s", roomID), http.StatusFound)
}

// Middleware для проверки авторизации
func WithAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session")
		token, ok := session.Values["token"].(string)
		if !ok || token == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		claims, err := auth.ValidateJWT(token)
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
type InitHandler struct {
	File *file.File
}

func (i *InitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tpl := template.Must(
		template.ParseFiles(filepath.Join("static", "editor.html")),
	)

	text, err := i.File.Read()
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
