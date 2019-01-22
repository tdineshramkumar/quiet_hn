package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/t-drk/quiet_hn_1/hn"
)

var (
	Cache       bool
	Parallelize bool
	TopStories  func(int) <-chan hn.Result
)

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.BoolVar(&Cache, "cache", false, "enable caching of hn")
	flag.BoolVar(&Parallelize, "parallel", false, "enable parallel load of hn")
	flag.Parse()
	fmt.Printf("Server Configuration: Caching:%v Parallelize:%v", Cache, Parallelize)
	if Parallelize {
		TopStories = TopStoriesParallel
	} else {
		TopStories = TopStoriesSequential
	}
	tpl := template.Must(template.ParseFiles("./index.gohtml"))
	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {
	request := make(chan chan []Item)
	if !Cache {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			result := <-TopStories(numStories)
			if result.Error != nil {
				http.Error(w, "Failed to load top stores", http.StatusInternalServerError)
				return
			}

			stories := result.Value.([]Item)
			data := templateData{
				Stories: stories,
				Time:    time.Now().Sub(start),
			}
			err := tpl.Execute(w, data)
			if err != nil {
				http.Error(w, "Failed to process the template", http.StatusInternalServerError)
				return
			}
		})
	}
	go loadTopStories(numStories, request)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fmt.Println("Request", r)
		start := time.Now()
		response := make(chan []Item)
		request <- response
		stories := <-response
		if stories == nil {
			http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
			return
		}
		data := templateData{
			Stories: stories,
			Time:    time.Now().Sub(start),
		}
		err := tpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Failed to process the template", http.StatusInternalServerError)
			return
		}

	})
}

// Refresh and Expire Time in case Server Caches
const (
	expireTime  = 900 // in seconds
	refreshTime = 600 // in seconds
)

// loadTopStories is an asynchronous task which maintains the top stories cache and
// responds to input requests
func loadTopStories(numStories int, request <-chan chan []Item) {
	var response chan []Item
	var Error error
	var expire, refresh <-chan time.Time
	var cache []Item
	stories := TopStories(numStories)
	for {
		input := request
		if cache == nil && Error == nil {
			input = nil
		}
		select {
		case result := <-stories:
			stories = nil
			expire = time.After(expireTime * time.Second)
			refresh = time.After(refreshTime * time.Second)
			if result.Error != nil {
				Error = result.Error
				cache = nil
				fmt.Println("Updates to top stories FAILURE")
				break
			}
			Error = nil
			cache = result.Value.([]Item)
			fmt.Println("Updates to top stories SUCCESS")
		case response = <-input:
			fmt.Println("New Request Made")
			go func() {
				if Error == nil {
					response <- cache
				} else {
					response <- nil
				}
			}()
		case <-expire:
			fmt.Println("Cache Expired.")
			Error = nil
			cache = nil
			stories = TopStories(numStories)
			expire = nil
			refresh = nil
		case <-refresh:
			fmt.Println("Cache Refreshed.")
			if stories == nil {
				stories = TopStories(numStories)
			}
			refresh = nil
		}

	}

}

func TopStoriesSequential(numStories int) <-chan hn.Result {
	out := make(chan hn.Result)
	go func() {
		start := time.Now()
		client := new(hn.Client)
		topItems := client.TopItems()
		if topItems.Error != nil {
			out <- hn.Result{nil, topItems.Error}
			return
		}
		ids := topItems.Value.([]int)
		stories := make([]Item, 0, numStories)
		for _, id := range ids {
			result := client.GetItem(id)
			item := result.Value.(hn.Item)
			if result.Error == nil && IsStoryLink(item) {
				stories = append(stories, ParseHNItem(item))
				if len(stories) >= numStories {
					break
				}
			}
		}
		fmt.Println("Took time to Get Top Stories Sequentially \n.", time.Now().Sub(start))
		out <- hn.Result{stories, nil}
	}()
	return out

}

// Parallel Server configuration variables
const (
	numRoutines = 50
	replication = 2
)

// TopStoriesParallel fetches hn top stories asynchronously and returns them through a channel
func TopStoriesParallel(numStories int) <-chan hn.Result {
	out := make(chan hn.Result)
	go func() {
		start := time.Now()
		c := new(hn.Client)
		topItems := multiCaller(c.TopItems, replication)
		fmt.Println("Got Top Items.")
		if topItems.Error != nil {
			out <- hn.Result{nil, topItems.Error}
			return
		}
		quit := make(chan bool)
		items := make(chan hn.Result)
		ids := generator(topItems.Value.([]int), quit)
		for i := 0; i < numRoutines; i++ {
			go processor(c, ids, items, quit)
		}
		fmt.Println("Waiting for Aggregator")
		stories := aggregator(numStories, topItems.Value.([]int), items, quit)
		fmt.Println("Took time to Get Top Stories IN Parallel\n.", time.Now().Sub(start))
		out <- hn.Result{stories, nil}
	}()
	return out
}

// generator converts a list to a channel
func generator(values []int, quit <-chan bool) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, value := range values {
			select {
			case out <- value:
			case <-quit:
				return
			}
		}
	}()
	return out
}

// processor fetches the stories for ids from the channel
func processor(c *hn.Client, ids <-chan int, out chan<- hn.Result, quit <-chan bool) {
	var results []hn.Result
	for {
		var send chan<- hn.Result
		var first hn.Result
		if len(results) > 0 {
			send = out
			first = results[0]
		}
		select {
		case id, ok := <-ids:
			if !ok {
				// Don't Send
				// Wait for processed items to be sent..
				fmt.Println("No More IDS to read")
				ids = nil
				continue
			}
			result := multiCaller(func() hn.Result { return c.GetItem(id) }, replication)
			results = append(results, result)
		case send <- first:
			results = results[1:]
		case <-quit:
			return
		}
	}
}

// aggregator collects the results from the channel and returns the result when top num_stories
// are obtained
func aggregator(numStories int, ids []int, items <-chan hn.Result, quit chan<- bool) []Item {
	defer close(quit)
	valid := make(map[int]bool)
	got := make(map[int]hn.Item)
	var stories []Item
	for result := range items {
		item := result.Value.(hn.Item)
		if result.Error != nil || !IsStoryLink(item) {
			valid[item.ID] = false
		} else {
			valid[item.ID] = true
			got[item.ID] = item
		}
		count := 0
		for _, id := range ids {
			if _, ok := valid[id]; !ok {
				break
			}
			if valid[id] {
				count++
			}
		}
		fmt.Printf("[C:%d]", count)
		if count >= numStories {
			i := 0
			for _, id := range ids {
				if valid[id] {
					stories = append(stories, ParseHNItem(got[id]))
					i++
					if i >= numStories {
						break
					}
				}
			}
			break
		}
	}
	return stories

}

const multiCallerTimeout = 10 // the timeout (seconds) for multicaller so that it can retry
// multiCaller runs the given function in mutliple goroutines and
// returns the return value of the fastest gorouting
func multiCaller(f func() hn.Result, n int) hn.Result {
	if n <= 1 {
		return f()
	}
	results := make(chan hn.Result)
	done := make(chan bool)

	for i := 0; i < n; i++ {
		go func(results chan<- hn.Result, done <-chan bool) {
			result := f()
			select {
			case results <- result:
			case <-done:
			}
		}(results, done)
	}
	// TODO: For robustness later check if the result contains an error value
	// and wait for later results
	select {
	case result := <-results:
		close(done)
		return result
	case <-time.After(multiCallerTimeout * time.Second):
		close(done)
		fmt.Println("ReCalling MultiCaller Due to Extensive Delays.")
		return multiCaller(f, n)
	}
}

func IsStoryLink(hnItem hn.Item) bool {
	return hnItem.Type == "story" && hnItem.URL != ""
}

func ParseHNItem(hnItem hn.Item) Item {
	ret := Item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

// Item is the same as the hn.Item but adds the Host field
type Item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []Item
	Time    time.Duration
}
