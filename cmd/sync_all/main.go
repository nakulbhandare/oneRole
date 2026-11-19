package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

type Company struct {
	Company_name string `json:"company_name"`
	Base_job_url string `json:"base_job_url"`
	Ats          string `json:"ats"`
}

func main() {
	file, err := os.Open("config/master_companies.json")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	var companies []Company
	if err := json.NewDecoder(file).Decode(&companies); err != nil {
		fmt.Println("Error decoding JSON:", err)
		return
	}

	var wg sync.WaitGroup
	const workerCount = 5
	jobs := make(chan string, len(companies))

	// Start workers
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for companyName := range jobs {
				fmt.Printf("Syncing %s...\n", companyName)
				cmd := exec.Command("./server", "discover", "--company", companyName)
				if out, err := cmd.CombinedOutput(); err != nil {
					fmt.Printf("Error syncing %s: %s\n", companyName, out)
				}
			}
		}()
	}

	for _, c := range companies {
		if c.Company_name != "" {
			jobs <- c.Company_name
		}
	}
	close(jobs)
	wg.Wait()
}
