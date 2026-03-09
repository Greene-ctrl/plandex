package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

func FinalizeProjectHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for FinalizeProjectHandler")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading finalize request body: %v\n", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	log.Printf("Finalized Project Data: %s", string(body))

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err == nil {
		if overview, ok := data["overview"].(string); ok {
			log.Printf("Received Project Overview:\n%s", overview)
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Project finalized and received successfully"))
}

func ExampleProjectOverviewHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for ExampleProjectOverviewHandler")

	exampleOverview := `# Project Overview: Example Smart Home System

## 1. System Architecture (Mermaid)
` + "```mermaid" + `
graph TD
    A[Mobile App] -->|HTTPS| B[Home Gateway]
    B -->|FastAPI| C[Device Service]
    B -->|FastAPI| D[User Service]
    C -->|MQTT| E[Smart Lights]
    C -->|MQTT| F[Smart Thermostat]
` + "```" + `

## 2. Component Details & FastAPI Interactions
- **Component**: Home Gateway
  - **Purpose**: Entry point for all mobile app requests. Handles routing and high-level logic.
  - **Endpoints**:
    - ` + "`" + `POST /api/v1/devices/control` + "`" + `: Route control commands to Device Service.
- **Component**: Device Service
  - **Purpose**: Manages connections to IoT devices and stores their current state.
  - **Endpoints**:
    - ` + "`" + `GET /api/v1/devices` + "`" + `: List all connected devices.
    - ` + "`" + `PATCH /api/v1/devices/{id}` + "`" + `: Update device state (e.g., turn light on).

## 3. GitHub Repository Tasks
### Repository: main-home-gateway
- [ ] Implement Home Gateway FastAPI server.
- [ ] Integrate Device Service client using HTTPS/FastAPI.
- [ ] Set up Docker Compose for local development.

### Repository: device-service-microservice
- [ ] Create Device Service backend using FastAPI.
- [ ] Implement MQTT client for device communication.
- [ ] Add unit tests for state management logic.

## 4. Deployment Strategy
- **Strategy**: Distributed HTTPS
- **Reasoning**: The Device Service needs to scale independently and may be deployed on edge hardware closer to the IoT devices, while the Home Gateway runs in a central cloud environment.

<PlandexFinish/>
`

	w.Header().Set("Content-Type", "text/markdown")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(exampleOverview))
}
