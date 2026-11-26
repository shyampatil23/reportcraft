package main

import (
	"embed"
	"flag"
	"fmt"
	"log"

	"performancereport/server"
	"performancereport/services"
	"performancereport/storage"
)

//go:embed frontend/dist
var frontendFS embed.FS

//go:embed assets/ERA_Master_Template.docx
var eraMasterTemplateBytes []byte

func main() {
	services.ERAMasterTemplateBytes = eraMasterTemplateBytes
	port := flag.String("port", "8765", "Port to listen on")
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lshortfile)
	fmt.Printf("Starting Performance Report Generator on port %s\n", *port)
	fmt.Printf("Data directory: %s\n", storage.DataPath())

	server.Start(*port, frontendFS)
}
