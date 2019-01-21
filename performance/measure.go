package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func main() {
	// parse flags
	var port, numTrials int
	flag.IntVar(&port, "port", 3000, "the port in which web server is running")
	flag.IntVar(&numTrials, "num_trials", 1, "the number of times to query for request")
	flag.Parse()

	server := fmt.Sprintf("http://localhost:%d/", port)
	var success_time, failure_time []time.Duration
	for i := 0; i < numTrials; i++ {
		start := time.Now()
		resp, err := http.Get(server)
		responseTime := time.Now().Sub(start)
		fmt.Println("Response Time:", responseTime)
		if err != nil {
			fmt.Println("ERROR:", err)
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Println("ERROR:", err)
			continue
		}
		contents := string(body)
		fmt.Println("Status Code", resp.Status)
		if resp.StatusCode == 200 {
			success_time = append(success_time, responseTime)
			fmt.Println("SUCCESSFUL RESPONSE")
		} else {
			failure_time = append(failure_time, responseTime)
			fmt.Println("FAILURE: ", contents)
		}
	}
	fmt.Println("Num Trials:", numTrials)
	if success_time != nil {
		min, max, avg := parameters(success_time)
		fmt.Printf("SUCCESSFUL RESPONSE AVG:%v MIN:%v MAX:%v FOR %v VALUES.\n", avg, min, max, len(success_time))
	}
	if failure_time != nil {
		min, max, avg := parameters(failure_time)
		fmt.Printf("FAILURE RESPONSE AVG:%v MIN:%v MAX:%v FOR %v VALUES.\n", avg, min, max, len(failure_time))
	}
}

// parameters computes maximum, minimum and average response time in seconds
func parameters(measures []time.Duration) (min, max, avg float64) {
	if measures == nil {
		panic("Measures is nil")
	}
	sum := float64(0)
	for _, d := range measures {
		s := d.Seconds()
		if min > s {
			min = s
		}
		if max < s {
			max = s
		}
		sum += s
	}
	avg = sum / float64(len(measures))
	return
}
