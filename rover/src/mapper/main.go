package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	nmap "github.com/Ullaakut/nmap/v3"
)

// ─── API response types ───────────────────────────────────────────────────────

type GatewayResponse struct {
	Colony      string     `json:"colony"`
	Established string     `json:"established"`
	Population  int        `json:"population"`
	Status      string     `json:"status"`
	Message     string     `json:"message"`
	Entrypoint  Entrypoint `json:"entrypoint"`
}

type Entrypoint struct {
	Pod         string `json:"pod"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type PodInfo struct {
	Name       string                 `json:"name"`
	ID         string                 `json:"id"`
	Role       string                 `json:"role"`
	Population int                    `json:"population"`
	Status     string                 `json:"status"`
	UptimeDays int                    `json:"uptime_days"`
	Metadata   map[string]interface{} `json:"metadata"`
}

type DependencyList struct {
	ID           string       `json:"id"`
	Dependencies []Dependency `json:"dependencies"`
}

type Dependency struct {
	PodID       string `json:"pod_id"`
	Resource    string `json:"resource"`
	Criticality string `json:"criticality"`
	Notes       string `json:"notes"`
}

type SupplyList struct {
	ID       string   `json:"id"`
	Supplies []Supply `json:"supplies"`
}

type Supply struct {
	PodID    string `json:"pod_id"`
	Resource string `json:"resource"`
}

type PodStatus struct {
	ID           string      `json:"id"`
	Status       string      `json:"status"`
	Alerts       []string    `json:"alerts"`
	LastIncident interface{} `json:"last_incident"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	Detail    string `json:"detail"`
}

type LogList struct {
	ID   string     `json:"id"`
	Logs []LogEntry `json:"logs"`
}

type CommMessage struct {
	Timestamp string `json:"timestamp"`
	From      string `json:"from"`
	To        string `json:"to"`
	Message   string `json:"message"`
}

type CommList struct {
	ID       string        `json:"id"`
	Messages []CommMessage `json:"messages"`
}

// ─── Output types ─────────────────────────────────────────────────────────────

type PodRecord struct {
	ID           string          `json:"id"`
	URL          string          `json:"url"`
	Info         *PodInfo        `json:"info"`
	Dependencies *DependencyList `json:"dependencies"`
	Supplies     *SupplyList     `json:"supplies"`
	Status       *PodStatus      `json:"status"`
	Logs         *LogList        `json:"logs"`
	Comms        *CommList       `json:"comms,omitempty"`
}

type MapOutput struct {
	Colony     string          `json:"colony"`
	Status     string          `json:"status"`
	Population int             `json:"population"`
	ScannedAt  string          `json:"scanned_at"`
	Gateway    GatewayResponse `json:"gateway"`
	Pods       []PodRecord     `json:"pods"`
}

// ─── Regex ────────────────────────────────────────────────────────────────────

var podIDRegex = regexp.MustCompile(`^project-selene-([a-z]+)-\d+`)

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		fatalf("GATEWAY_URL environment variable is required")
	}
	outputDir := "/rover/output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fatalf("create output dir: %v", err)
	}

	// Phase 1: nmap scan
	fmt.Println("=== Phase 1: Network Discovery ===")
	subnet, err := detectSubnet()
	if err != nil {
		fatalf("detect subnet: %v", err)
	}
	fmt.Printf("Scanning %s ...\n", subnet)
	podPort, err := scanNetwork(subnet)
	if err != nil {
		fatalf("scan: %v", err)
	}
	fmt.Println("Scan complete.")

	// Phase 2: print discovered service map
	fmt.Println("=== Phase 2: Building service map ===")
	for id, port := range podPort {
		fmt.Printf("  %s -> :%s\n", id, port)
	}

	// Phase 3: gateway handshake
	fmt.Println("=== Phase 3: Gateway handshake ===")
	client := &http.Client{Timeout: 10 * time.Second}
	gateway, err := fetchJSON[GatewayResponse](client, gatewayURL)
	if err != nil {
		fatalf("gateway: %v", err)
	}
	fmt.Printf("Colony:     %s\n", gateway.Colony)
	fmt.Printf("Status:     %s\n", gateway.Status)
	fmt.Printf("Entrypoint: %s\n", gateway.Entrypoint.Pod)

	// Phase 4: BFS crawl
	fmt.Println("=== Phase 4: Crawling pods ===")
	visited := map[string]bool{}
	queue := []string{gateway.Entrypoint.Pod}
	visited[gateway.Entrypoint.Pod] = true
	var pods []PodRecord

	for len(queue) > 0 {
		podID := queue[0]
		queue = queue[1:]

		port, ok := podPort[podID]
		if !ok {
			fmt.Printf("  [WARN] No port known for '%s' — skipping\n", podID)
			continue
		}

		url := fmt.Sprintf("http://%s:%s", podID, port)
		fmt.Printf("  -> %s (%s)\n", podID, url)

		info, _ := fetchJSON[PodInfo](client, url+"/info")
		deps, _ := fetchJSON[DependencyList](client, url+"/dependencies")
		supps, _ := fetchJSON[SupplyList](client, url+"/supplies")
		status, _ := fetchJSON[PodStatus](client, url+"/status")
		logs, _ := fetchJSON[LogList](client, url+"/logs")
		comms, _ := fetchJSON[CommList](client, url+"/comms") // 404 on most pods — nil is fine

		pods = append(pods, PodRecord{
			ID:           podID,
			URL:          url,
			Info:         info,
			Dependencies: deps,
			Supplies:     supps,
			Status:       status,
			Logs:         logs,
			Comms:        comms,
		})

		// Enqueue unvisited neighbours from dependencies
		if deps != nil {
			for _, d := range deps.Dependencies {
				if !visited[d.PodID] {
					visited[d.PodID] = true
					queue = append(queue, d.PodID)
					fmt.Printf("     queued (dep): %s\n", d.PodID)
				}
			}
		}
		// Enqueue unvisited neighbours from supplies
		if supps != nil {
			for _, s := range supps.Supplies {
				if !visited[s.PodID] {
					visited[s.PodID] = true
					queue = append(queue, s.PodID)
					fmt.Printf("     queued (supply): %s\n", s.PodID)
				}
			}
		}
	}

	// Phase 5: write map.json
	fmt.Println("=== Phase 5: Writing map.json ===")
	out := MapOutput{
		Colony:     gateway.Colony,
		Status:     gateway.Status,
		Population: gateway.Population,
		ScannedAt:  time.Now().UTC().Format(time.RFC3339),
		Gateway:    *gateway,
		Pods:       pods,
	}
	f, err := os.Create(filepath.Join(outputDir, "map.json"))
	if err != nil {
		fatalf("create map.json: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fatalf("encode map.json: %v", err)
	}
	fmt.Printf("Done. Discovered %d pods.\n", len(pods))
	fmt.Printf("Map saved to %s/map.json\n", outputDir)
}

// ─── Network helpers ─────────────────────────────────────────────────────────

// detectSubnet returns the CIDR of the first non-loopback IPv4 interface.
func detectSubnet() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				return ipNet.String(), nil
			}
		}
	}
	return "", fmt.Errorf("could not detect subnet from network interfaces")
}

// ─── nmap helpers ─────────────────────────────────────────────────────────────

func scanNetwork(subnet string) (map[string]string, error) {
	scanner, err := nmap.NewScanner(
		context.Background(),
		nmap.WithTargets(subnet),
		nmap.WithCustomArguments("-sT", "-p-", "-T4"),
	)
	if err != nil {
		return nil, fmt.Errorf("create scanner: %w", err)
	}

	result, warnings, err := scanner.Run()
	for _, w := range *warnings {
		fmt.Printf("  [nmap warn] %s\n", w)
	}
	if err != nil {
		return nil, fmt.Errorf("nmap run: %w", err)
	}

	podPort := map[string]string{}
	for _, host := range result.Hosts {
		var podID string
		for _, hn := range host.Hostnames {
			if m := podIDRegex.FindStringSubmatch(hn.Name); m != nil {
				podID = m[1]
				break
			}
		}
		if podID == "" {
			continue
		}
		for _, p := range host.Ports {
			if p.State.State == "open" {
				podPort[podID] = strconv.Itoa(int(p.ID))
				break
			}
		}
	}
	return podPort, nil
}

// ─── HTTP helper ──────────────────────────────────────────────────────────────

func fetchJSON[T any](client *http.Client, url string) (*T, error) {
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}
