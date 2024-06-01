package main

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/juliazadorozhnaya/OnlineTextEditor/auth"
	"github.com/juliazadorozhnaya/OnlineTextEditor/file"
	"github.com/juliazadorozhnaya/OnlineTextEditor/handler"
	"github.com/juliazadorozhnaya/OnlineTextEditor/websocket"
	"net/http"
	"os"
	"path/filepath"
)

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

	file := file.New(filepath.Join("data", "text.txt"))
	h := websocket.NewHub(file)
	go h.Run()

	r := mux.NewRouter()
	r.HandleFunc("/", handler.ShowHomePage).Methods("GET")
	r.HandleFunc("/login", handler.HandleLogin).Methods("POST")
	r.HandleFunc("/ws/{roomID}", func(w http.ResponseWriter, r *http.Request) {
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
		r = r.WithContext(context.WithValue(r.Context(), "username", claims.Username))
		websocket.ServeWs(h, file, w, r)
	})
	r.Handle("/editor/{roomID}", handler.WithAuth(&handler.InitHandler{File: file})).Methods("GET")

	if err := http.ListenAndServe(":8080", r); err != nil {
		return err
	}

	return nil
}
