package main

import (
	"flag"
	"fmt"
	"hackernews/hn"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type FetchMode int

const (
	FetchModeVanilla FetchMode = iota
	FetchModeConcurrent
	FetchModeConcurrentWithCache
)
const (
	HN_ENDPOINT = "https://hacker-news.firebaseio.com/v0"
	BATCH_SIZE  = 8
)

type item struct {
	idx int
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func hiHandler(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, map[string]string{
		"message": "hello there!",
	})
}
func feedHandler(mode FetchMode, numStories *int) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		client := hn.New(HN_ENDPOINT)
		start := time.Now()
		ids, err := client.TopItems()

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, err)
		}
		feedConfigs := &feedConfigs{
			Mode:       mode,
			NumStories: *numStories,
			IDs:        ids,
			Client:     hn.New(HN_ENDPOINT),
		}
		stories, err := fetchTopStories(feedConfigs)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, err)
		}
		data := templateData{
			Stories: stories,
			Time:    time.Since(start),
		}
		ctx.HTML(http.StatusOK, "index.html", gin.H{
			"data": data,
		})
	}
}

func performConcurrentFetch(config *feedConfigs, ids []int) []item {
	type result struct {
		idx  int
		err  error
		item item
	}
	resultChannel := make(chan result)

	for i := 0; i < len(ids); i++ {
		go func(id, idx int) {
			fmt.Printf("GettingItem [%v]: Start\n", id)
			hnItem, err := config.Client.GetItem(id, config.Mode == FetchModeConcurrent)
			if err != nil {
				resultChannel <- result{idx: idx, err: err}
			}
			resultChannel <- result{idx: idx, item: parseHNItem(hnItem)}
			fmt.Printf("GettingItem [%v]: Done\n", id)

		}(ids[i], i)
	}

	var results []result
	for i := 0; i < len(ids); i++ {
		results = append(results, <-resultChannel)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].idx < results[j].idx
	})

	var items []item
	for _, result := range results {
		if result.err == nil && isStoryLink(result.item) {
			items = append(items, result.item)
		}
	}
	return items
}

func fetchConcurrent(config *feedConfigs) ([]item, error) {
	var stories []item
	start := 0
	for len(stories) < config.NumStories {
		needed := (config.NumStories - len(stories))
		end := start + needed
		newStories := performConcurrentFetch(config, config.IDs[start:end])
		stories = append(stories, newStories...)
		start = end
	}
	return stories, nil
}
func fetchVanilla(config *feedConfigs) ([]item, error) {
	var stories []item
	for _, id := range config.IDs {
		hnItem, err := config.Client.GetItem(id, true)
		if err != nil {
			continue
		}
		item := parseHNItem(hnItem)
		if isStoryLink(item) {
			stories = append(stories, item)
			if len(stories) >= config.NumStories {
				break
			}
		}
	}
	return stories, nil
}

func fetchTopStories(config *feedConfigs) ([]item, error) {
	if config.Mode == FetchModeVanilla {
		return fetchVanilla(config)
	}
	return fetchConcurrent(config)
}

type feedConfigs struct {
	Mode       FetchMode
	NumStories int
	IDs        []int
	Client     *hn.Client
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	numStories := flag.Int("limit", 20, "number of hacker news stories to fetch")
	flag.Parse()

	// typical fetch
	r.GET("/feed", feedHandler(FetchModeVanilla, numStories))
	// fetch concurrently
	r.GET("/feedc", feedHandler(FetchModeConcurrent, numStories))
	// fetch concurrently with cache
	r.GET("/feedcc", feedHandler(FetchModeConcurrentWithCache, numStories))
	// check
	r.GET("/hi", hiHandler)
	r.Run()
}
