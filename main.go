package main


import (
	"flag"
	"fmt"
	"github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
	"log"
)

type NomadClient struct {
	NodeID   string `json:"NodeID"`
	NodeName string `json:"NodeName"`
	JobStatus bool `json:"JobStatus"`
}

type NomadClients []NomadClient

func main() {
	// get flags from cli
	addr := flag.String("addr", "http://localhost:4646", "NOMAD_ADDR")
	token := flag.String("token", "asdf1234-asdf1234-asdf1234", "NOMDAD_TOKEN")
	jobName := flag.String("job-name", "signoz-logspout", "Nomad job name to monitor")

	// Parse flags
	flag.Parse()


	fmt.Println("Starting Nomad Logspout Client Monitor...")

	// Get all the available nomad clients that should be running logspout
	nomad, err := api.NewClient(&api.Config{
		Address: *addr,
		SecretID: *token,		
		TLSConfig: &api.TLSConfig{
			Insecure: true,
		},
	})
	if err != nil {
		fmt.Println("Error creating Nomad client: ", err)
	}

	var Clients NomadClients
	// Get the nomad clients
	nomadClients, err := GetClientList(nomad)
	if err != nil {
		fmt.Println("Error getting nomad clients: ", err)
	}
	
	Clients, err = CheckNomadJobStatus(nomad, nomadClients, jobName)
	if err != nil {
		fmt.Println("Error checking nomad job status: ", err)
	}

	// Print the nomad clients
	for _, client := range Clients {
		fmt.Printf("NodeID: %s, NodeName: %s, JobStatus: %t\n", client.NodeID, client.NodeName, client.JobStatus)
	}

		initMetrics()

	// Refresh metrics every 30s
	go func() {
		for {
			updateClientStatus(nomad, jobName)
			time.Sleep(1 * time.Hour)
		}
	}()

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	fmt.Println("Serving Prometheus metrics on :2112/metrics")
	log.Fatal(http.ListenAndServe(":2112", nil))
}

func GetClientList(nomad *api.Client) ([]*api.NodeListStub, error) {
	clients, _, err := nomad.Nodes().List(nil)
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func CheckNomadJobStatus(nomad *api.Client, clientsList []*api.NodeListStub, jobName *string) (NomadClients, error) {
	var Clients NomadClients
	for _, client := range clientsList {
		var jobStatus bool

		allocations, _, err := nomad.Jobs().Allocations(*jobName, true, nil)
		if err != nil {
			fmt.Println("Error getting allocations: ", err)
		}
		for _, allocation := range allocations {
			if allocation.NodeName == client.Name {
				jobStatus = true
			} 
		}
		
		Clients = append(Clients, NomadClient{
			NodeID:   client.ID,
			NodeName: client.Name,
			JobStatus: jobStatus,
		})
		jobStatus = false
	}

	return Clients, nil
}


var (
	nodeStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "logspout_client_up",
			Help: "Nomad logspout client node status (1 = job is running, 0 = job is not running)",
		},
		[]string{"ClientID", "ClientName"},
	)

	registry = prometheus.NewRegistry()
)

func initMetrics() {
	registry.MustRegister(nodeStatusGauge)
}

func updateClientStatus(nomad *api.Client, jobName *string) {
	now := time.Now()
	fmt.Println(now.Format(time.RFC3339) , " - Updating client status...")
	clients, err := GetClientList(nomad)
	time.Sleep(2 * time.Second)
	if err != nil {
		fmt.Println("Error getting nomad clients: ", err)
	}

	for _, client := range clients {
		jobStatus := false
		allocations, _, err := nomad.Jobs().Allocations(*jobName, true, nil)
		if err != nil {
			fmt.Println("Error getting allocations: ", err)
		}
		for _, allocation := range allocations {
			if allocation.NodeName == client.Name {
				jobStatus = true
			} 
		}
		
		if jobStatus {
			nodeStatusGauge.WithLabelValues(client.ID, client.Name).Set(1)
		} else {
			nodeStatusGauge.WithLabelValues(client.ID, client.Name).Set(0)
		}
	}
}
	
