package solver

import (
	"log"
	"os"
)

var (
	Logger *log.Logger // Global logger instance
)

// InitLogger initializes the logger with custom configuration
func InitLogger() {
	// Create a logger with timestamp, file location, and line number
	Logger = log.New(os.Stdout, "NFT-CLIENT: ", log.Ldate|log.Ltime|log.Lshortfile)
}
