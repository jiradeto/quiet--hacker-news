package hn

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	baseEndpoint string
}

func New(baseEndpoint string) *Client {
	return &Client{
		baseEndpoint,
	}
}

var cache = make(map[int]Item)

func (c *Client) GetItem(id int, forceFetch bool) (Item, error) {
	if !forceFetch {
		item, exist := cache[id]
		if exist {
			return item, nil
		}
	}
	var item Item
	resp, err := http.Get(fmt.Sprintf("%s/item/%d.json", c.baseEndpoint, id))
	if err != nil {
		return item, err
	}

	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&item)
	if err == nil {
		cache[id] = item
	}
	return item, err
}

func (c *Client) TopItems() ([]int, error) {
	resp, err := http.Get(fmt.Sprintf("%s/topstories.json", c.baseEndpoint))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	var ids []int
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&ids)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

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
