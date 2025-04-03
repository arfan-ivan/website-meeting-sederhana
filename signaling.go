package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Client represents a connected user
type Client struct {
	ID   string
	Conn *websocket.Conn
}

// Room contains all clients in a meeting room
type Room struct {
	Clients map[string]*Client
	Mutex   sync.Mutex
}

// Message structure that matches the frontend
type Message struct {
	Type     string          `json:"type"`
	Room     string          `json:"room"`
	SDP      string          `json:"sdp,omitempty"`
	Sender   string          `json:"sender,omitempty"`
	Receiver string          `json:"receiver,omitempty"`
	UserList []string        `json:"userList,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

var (
	rooms   = make(map[string]*Room)
	roomsMu sync.Mutex
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	
	clientID := uuid.New().String()
	client := &Client{
		ID:   clientID,
		Conn: conn,
	}
	
	log.Printf("New client connected: %s", clientID)
	
	defer func() {
		handleClientDisconnect(client)
		conn.Close()
		log.Printf("Client disconnected: %s", clientID)
	}()
	
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}
		
		log.Printf("Received message: %+v", msg)
		
		// Set sender ID
		msg.Sender = clientID
		
		switch msg.Type {
		case "createRoom":
			handleCreateRoom(client, msg)
		case "joinRoom":
			handleJoinRoom(client, msg)
		case "offer":
			handleOffer(client, msg)
		case "answer":
			handleAnswer(client, msg)
		case "candidate":
			handleCandidate(client, msg)
		case "startCall":
			handleStartCall(client, msg)
		case "leaveRoom":
			handleLeaveRoom(client, msg)
		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func handleCreateRoom(client *Client, msg Message) {
	roomID := msg.Room
	
	roomsMu.Lock()
	if _, exists := rooms[roomID]; !exists {
		rooms[roomID] = &Room{
			Clients: make(map[string]*Client),
		}
	}
	
	room := rooms[roomID]
	roomsMu.Unlock()
	
	room.Mutex.Lock()
	room.Clients[client.ID] = client
	room.Mutex.Unlock()
	
	// Notify client that room was created
	response := Message{
		Type: "roomCreated",
		Room: roomID,
	}
	
	client.Conn.WriteJSON(response)
	log.Printf("Room created: %s by client %s", roomID, client.ID)
}

func handleJoinRoom(client *Client, msg Message) {
	roomID := msg.Room
	
	roomsMu.Lock()
	room, exists := rooms[roomID]
	if !exists {
		rooms[roomID] = &Room{
			Clients: make(map[string]*Client),
		}
		room = rooms[roomID]
	}
	roomsMu.Unlock()
	
	room.Mutex.Lock()
	room.Clients[client.ID] = client
	
	// Get list of users in room
	userList := make([]string, 0, len(room.Clients))
	for id := range room.Clients {
		if id != client.ID {
			userList = append(userList, id)
		}
	}
	room.Mutex.Unlock()
	
	// Inform the new user about the room join
	joinResponse := Message{
		Type:     "roomJoined",
		Room:     roomID,
		UserList: userList,
	}
	client.Conn.WriteJSON(joinResponse)
	
	// Notify other clients about new user
	newUserMsg := Message{
		Type:   "newUser",
		Room:   roomID,
		Sender: client.ID,
	}
	broadcastToRoom(roomID, newUserMsg, client.ID)
	
	log.Printf("Client %s joined room %s with %d existing users", client.ID, roomID, len(userList))
}

func handleStartCall(client *Client, msg Message) {
	roomID := msg.Room
	
	roomsMu.Lock()
	room, exists := rooms[roomID]
	roomsMu.Unlock()
	
	if !exists {
		log.Printf("Room %s not found", roomID)
		return
	}
	
	room.Mutex.Lock()
	defer room.Mutex.Unlock()
	
	// Tell everyone in the room to connect to the new user
	for userID, userClient := range room.Clients {
		if userID != client.ID {
			startCallMsg := Message{
				Type:     "initiatePeerConnection",
				Room:     roomID,
				Sender:   client.ID,
				Receiver: userID,
			}
			userClient.Conn.WriteJSON(startCallMsg)
		}
	}
	
	log.Printf("Call started in room %s by client %s", roomID, client.ID)
}

func handleOffer(client *Client, msg Message) {
	roomID := msg.Room
	targetID := msg.Receiver
	
	roomsMu.Lock()
	room, exists := rooms[roomID]
	roomsMu.Unlock()
	
	if !exists {
		log.Printf("Room %s not found", roomID)
		return
	}
	
	room.Mutex.Lock()
	targetClient, exists := room.Clients[targetID]
	room.Mutex.Unlock()
	
	if !exists {
		log.Printf("Target client %s not found in room %s", targetID, roomID)
		return
	}
	
	// Forward offer to target client
	targetClient.Conn.WriteJSON(msg)
	log.Printf("Forwarded offer from client %s to client %s", client.ID, targetID)
}

func handleAnswer(client *Client, msg Message) {
	roomID := msg.Room
	targetID := msg.Receiver
	
	roomsMu.Lock()
	room, exists := rooms[roomID]
	roomsMu.Unlock()
	
	if !exists {
		log.Printf("Room %s not found", roomID)
		return
	}
	
	room.Mutex.Lock()
	targetClient, exists := room.Clients[targetID]
	room.Mutex.Unlock()
	
	if !exists {
		log.Printf("Target client %s not found in room %s", targetID, roomID)
		return
	}
	
	// Forward answer to target client
	targetClient.Conn.WriteJSON(msg)
	log.Printf("Forwarded answer from client %s to client %s", client.ID, targetID)
}

func handleCandidate(client *Client, msg Message) {
	roomID := msg.Room
	targetID := msg.Receiver
	
	roomsMu.Lock()
	room, exists := rooms[roomID]
	roomsMu.Unlock()
	
	if !exists {
		log.Printf("Room %s not found", roomID)
		return
	}
	
	room.Mutex.Lock()
	targetClient, exists := room.Clients[targetID]
	room.Mutex.Unlock()
	
	if !exists {
		log.Printf("Target client %s not found in room %s", targetID, roomID)
		return
	}
	
	// Forward ICE candidate to target client
	targetClient.Conn.WriteJSON(msg)
	log.Printf("Forwarded ICE candidate from client %s to client %s", client.ID, targetID)
}

func handleLeaveRoom(client *Client, msg Message) {
	roomID := msg.Room
	handleClientLeaveRoom(client, roomID)
}

func handleClientLeaveRoom(client *Client, roomID string) {
	roomsMu.Lock()
	room, exists := rooms[roomID]
	roomsMu.Unlock()
	
	if !exists {
		return
	}
	
	room.Mutex.Lock()
	delete(room.Clients, client.ID)
	
	// If room is empty, remove it
	isEmpty := len(room.Clients) == 0
	room.Mutex.Unlock()
	
	if isEmpty {
		roomsMu.Lock()
		delete(rooms, roomID)
		roomsMu.Unlock()
		log.Printf("Room %s removed as it's empty", roomID)
	} else {
		// Notify others that user has left
		leaveMsg := Message{
			Type:   "userLeft",
			Room:   roomID,
			Sender: client.ID,
		}
		broadcastToRoom(roomID, leaveMsg, "")
		log.Printf("Client %s left room %s", client.ID, roomID)
	}
}

func handleClientDisconnect(client *Client) {
	// Find all rooms this client is in and remove them
	roomsMu.Lock()
	clientRooms := []string{}
	
	for roomID, room := range rooms {
		room.Mutex.Lock()
		if _, exists := room.Clients[client.ID]; exists {
			clientRooms = append(clientRooms, roomID)
		}
		room.Mutex.Unlock()
	}
	roomsMu.Unlock()
	
	// Remove client from all rooms
	for _, roomID := range clientRooms {
		handleClientLeaveRoom(client, roomID)
	}
}

func broadcastToRoom(roomID string, msg Message, excludeID string) {
	roomsMu.Lock()
	room, exists := rooms[roomID]
	roomsMu.Unlock()
	
	if !exists {
		return
	}
	
	room.Mutex.Lock()
	defer room.Mutex.Unlock()
	
	for clientID, client := range room.Clients {
		if clientID != excludeID {
			if err := client.Conn.WriteJSON(msg); err != nil {
				log.Printf("Error broadcasting to client %s: %v", clientID, err)
			}
		}
	}
}

func main() {
	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)
	
	// WebSocket endpoint
	http.HandleFunc("/ws", handleWebSocket)
	
	port := ":9220"
	fmt.Printf("Server running on http://172.16.1.2%s\n", port)
	log.Printf("WebSocket endpoint at ws://172.16.1.2%s/ws\n", port)
	
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}