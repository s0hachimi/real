package handler

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	db "real-time-forum/Database/cration"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "http://localhost:8080"
	},
}

var (
	clients      = make(map[*websocket.Conn]string)
	clientsMutex sync.RWMutex
	broadcast    = make(chan Message)
	typing       = make(chan Message)
)

type Message struct {
	Sender   string `json:"sender"`
	Receiver string `json:"receiver"`
	Content  string `json:"content"`
	Time     string
}

func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("WebSocket upgrade error:", err)
		return
	}
	defer func() {
		clientsMutex.Lock()
		delete(clients, conn)
		clientsMutex.Unlock()

		BroadcastUsers()
		BroadcastOnlineUsers()
		conn.Close()
	}()

	cookie, err := r.Cookie("SessionToken")

	if err != nil || cookie.Value == "" {
		fmt.Println("Error sessionToken ", err)
		return
	}

	username := db.GetUsernameByToken(cookie.Value)

	if username == "" {
		fmt.Println("this nigga does'nt exsist : ", username)
		return
	}

	clientsMutex.Lock()
	for existingConn, existingUsername := range clients {
		if existingUsername == username {
			fmt.Println("Closing previous connection for username:", username)
			existingConn.Close()
			delete(clients, existingConn)
			break
		}
	}
	clients[conn] = username
	clientsMutex.Unlock()
	BroadcastUsers()
	BroadcastOnlineUsers()

	for {

		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			fmt.Println("WebSocket read error:", err)
			break
		}

		if msg.Content == "is-typing" || msg.Content == "no-typing" {
			typing <- msg
		} else {
			time := time.Now().Format("2006-01-02 15:04:05")
			msg.Time = time

			err = db.InsertMessages(msg.Sender, msg.Receiver, msg.Content, msg.Time)
			if err != nil {
				fmt.Println("insert massages error:", err)
				return
			}
			broadcast <- msg
		}

	}
}

func HandleMessages() {
	for {
		msg := <-broadcast

		clientsMutex.RLock()
		for client, username := range clients {
			if username == msg.Receiver || username == msg.Sender {
				err := client.WriteJSON(msg)
				if err != nil {
					fmt.Println("WebSocket write error:", err)

					client.Close()

					clientsMutex.Lock()
					delete(clients, client)
					clientsMutex.Unlock()
				}
			}
		}
		clientsMutex.RUnlock()
	}
}

func BroadcastUsers() {
	clientsMutex.RLock()
	defer clientsMutex.RUnlock()

	allUsers, err := db.GetAllUsers()
	if err != nil {
		fmt.Println("Error fetching all users:", err)
		return
	}

	sortUsers, err := db.GetLastMessage(allUsers)
	if err != nil {
		fmt.Println("Error fetching all users:", err)
		return
	}

	users := []map[string]any{}
	for _, user := range sortUsers {
		online := false
		for _, onlineUser := range clients {
			if onlineUser == user.User {
				online = true
				break
			}
		}
		users = append(users, map[string]any{
			"username": user.User,
			"sort":     user.UserMsg,
			"online":   online,
			"allUsers": allUsers,
		})
	}

	message := map[string]any{
		"type":  "users",
		"users": users,
	}

	for client := range clients {
		err := client.WriteJSON(message)
		if err != nil {
			fmt.Println("WebSocket write error:", err)
			client.Close()
			clientsMutex.Lock()
			delete(clients, client)
			clientsMutex.Unlock()
		}
	}
}

func BroadcastOnlineUsers() {

	clientsMutex.RLock()
	defer clientsMutex.RUnlock()

	var online []string

	for _, client := range clients {
		online = append(online, client)
	}

	message := map[string]any{
		"type":  "online-users",
		"users": online,
	}

	for client := range clients {
		err := client.WriteJSON(message)
		if err != nil {
			fmt.Println("WebSocket write error:", err)
			client.Close()
			clientsMutex.Lock()
			delete(clients, client)
			clientsMutex.Unlock()
		}
	}
}

func Typing() {
	for {
		msg := <-typing

		clientsMutex.RLock()
		for client, username := range clients {
			if username == msg.Receiver || username == msg.Sender {
				err := client.WriteJSON(msg)
				if err != nil {
					fmt.Println(err)

					client.Close()

					clientsMutex.Lock()
					delete(clients, client)
					clientsMutex.Unlock()
				}
			}
		}
		clientsMutex.RUnlock()
	}
}
