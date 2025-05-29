package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type NomadClient struct {
	NodeID    string `json:"NodeID"`
	NodeName  string `json:"NodeName"`
	JobStatus bool   `json:"JobStatus"`
}

type NomadClients []NomadClient

func main() {
	// get flags from cli
	addr := flag.String("addr", "http://localhost:4646", "NOMAD_ADDR")
	token := flag.String("token", "asdf1234-asdf1234-asdf1234", "NOMDAD_TOKEN")
	jobName := flag.String("job-name", "signoz-logspout", "Nomad job name to monitor")

	insecure := flag.Bool("insecure", false, "Skip TLS verification")
	cert := flag.String("cert", "", "Path to the TLS certificate file")
	key := flag.String("key", "", "Path to the TLS key file")

	// Parse flags
	flag.Parse()

	config := api.DefaultConfig()
	config.Address = *addr
	config.SecretID = *token

	if *insecure {
		config.TLSConfig.Insecure = true
	} else {
		config.TLSConfig.Insecure = false
		config.TLSConfig.ClientCert = *cert
		config.TLSConfig.ClientKey = *key
	}

	fmt.Println("Starting Nomad Logspout Client Monitor...")

	// Get all the available nomad clients that should be running logspout
	nomad, err := api.NewClient(config)
	if err != nil {
		fmt.Println("Error creating Nomad client: ", err)
	}

	initMetrics()

	// Refresh metrics every 30s
	go func() {
		for {
			updateClientStatus(nomad, jobName)
			time.Sleep(30 * time.Second)
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
			NodeID:    client.ID,
			NodeName:  client.Name,
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
	fmt.Println(now.Format(time.RFC3339), " - Updating client status...")
	// -- get all the available nomad clients that should be running logspout
	nomadClients, err := GetClientList(nomad)
	if err != nil {
		fmt.Println("Error getting nomad clients: ", err)
		// -- if we can't get the clients, we will wait for 2 seconds and try again
		fmt.Println("Retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	} else {
		fmt.Println("Found", len(nomadClients), "nomad clients")
	}
	// -- get all the allocations for the specified job
	allocations, _, err := nomad.Jobs().Allocations(*jobName, true, nil)
	if err != nil {
		fmt.Println("Error getting allocations: ", err)
	} else {
		fmt.Println("Found", len(allocations), "allocations for job", *jobName)
	}
	// -- check that each client has the allocation, otherwise update to false
	for _, client := range nomadClients {
		jobStatus := false
		for _, allocation := range allocations {
			//fmt.Printf("NodeID: %s, NodeName: %s, ClientStatus: %s\n", client.ID, client.Name, allocation.ClientStatus)

			if allocation.NodeName == client.Name && allocation.ClientStatus == "running" {
				jobStatus = true
				break
			}
		}
		// -- update the metrics
		if jobStatus {
			nodeStatusGauge.WithLabelValues(client.ID, client.Name).Set(1)
		} else {
			nodeStatusGauge.WithLabelValues(client.ID, client.Name).Set(0)
		}
		fmt.Printf("NodeID: %s, NodeName: %s, JobStatus: %t\n", client.ID, client.Name, jobStatus)
	}
}
