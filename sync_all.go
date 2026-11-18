package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

	for _, c := range companies {
		if c.Company_name != "" {
			fmt.Printf("Syncing %s...\n", c.Company_name)
			cmd := exec.Command("./server", "discover", "--company", c.Company_name)
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("Error syncing %s: %s\n", c.Company_name, out)
			}
		}
	}
}
