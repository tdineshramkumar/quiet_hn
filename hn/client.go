package hn

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	apiBase = "https://hacker-news.firebaseio.com/v0"
)

// Client is an API client used to interact with the Hacker News API
type Client struct {
	//Nothing for now...
}

type Result struct {
	Value interface{}
	Error error
}

// TopItems returns the ids of roughlu 450 top items in decreasing order. These
// should map directly to the top 450 things you would see on HN if you visited
// these sites and kept going to the next page.
//
// TopItems does not filter out job listings or anything else, as the type of
// each item is unknown further API calls.
func (c *Client) TopItems() Result {
	resp, err := http.Get(fmt.Sprintf("%s/topstories.json", apiBase))
	if err != nil {
		return Result{nil, err}
	}
	defer resp.Body.Close()
	var ids []int
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&ids)
	if err != nil {
		return Result{nil, err}
	}
	return Result{ids, nil}
}

// GetItem wiil returh the Item defined by the provided ID.
func (c *Client) GetItem(id int) Result {
	var item Item
	defer func(item Item, id int) { item.ID = id }(item, id)
	resp, err := http.Get(fmt.Sprintf("%s/item/%d.json", apiBase, id))
	if err != nil {
		return Result{item, err}
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&item)
	if err != nil {
		return Result{item, err}
	}
	return Result{item, err}
}

// Item represents a single item returned by the HN API. This can have a type
// of "story", "comment", or "job" (and probably more values), and one of the
// URL or Text fields will be set, but not both.
//
// FOr the purpose of this exercise, we only care about items where the
// type is "story:, and the URL is set.
type Item struct {
	By          string `json:"by"`
	Descendants int    `json:"descendants"`
	ID          int    `json:"id"`
	Kids        []int  `json:"kids"`
	Score       int    `json:"score"`
	Time        int    `json:"time"`
	Title       string `json:"title"`
	Type        string `json:"type"`

	// Only one of these should exist
	Text string `json:"text"`
	URL  string `json:"url"`
}
