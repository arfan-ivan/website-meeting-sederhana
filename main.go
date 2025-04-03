package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pion/webrtc/v3"
)

func main() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello, WebRTC with Golang!")
	})

	// Endpoint untuk WebRTC signaling
	r.POST("/offer", func(c *gin.Context) {
		var req struct {
			SDP string `json:"sdp"`
		}

		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Buat PeerConnection WebRTC
		peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create PeerConnection"})
			return
		}

		// Set SDP dari klien
		offer := webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  req.SDP,
		}
		err = peerConnection.SetRemoteDescription(offer)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set remote description"})
			return
		}

		// Buat jawaban (answer) SDP
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create answer"})
			return
		}

		// Simpan LocalDescription ke PeerConnection
		err = peerConnection.SetLocalDescription(answer)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set local description"})
			return
		}

		// Kirimkan SDP answer ke klien
		c.JSON(http.StatusOK, gin.H{"sdp": answer.SDP})
	})

	fmt.Println("Server berjalan di http://localhost:8124")
	log.Fatal(r.Run(":8124"))
}

