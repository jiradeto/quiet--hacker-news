package main

import (
	"flag"
	"fmt"
	"hackernews/hn"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	HN_ENDPOINT = "https://hacker-news.firebaseio.com/v0"
	BATCH_SIZE  = 8
)

type item struct {
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

func feedConcurrentHandler(numStories *int, cacheEnabled bool) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		client := hn.New(HN_ENDPOINT)
		start := time.Now()
		ids, err := client.TopItems()
		forceFetch := true
		if cacheEnabled {
			forceFetch = false
		}

		rankingMap := map[int]int{}
		for i, id := range ids {
			rankingMap[id] = i
		}

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, err)
		}

		var stories []item
		batchSize := BATCH_SIZE
		for i := 0; i < len(ids); i += batchSize {
			j := i + batchSize
			if j > len(ids) {
				j = len(ids)
			}

			var wg sync.WaitGroup
			batchIds := ids[i:j]
			wg.Add(len(batchIds))
			for _, itemID := range batchIds {
				go func(id int) {
					defer wg.Done()
					fmt.Printf("[%v] - GettingItem: Start\n", id)
					hnItem, err := client.GetItem(id, forceFetch)
					if err != nil {
						fmt.Println("some error here", err)
					}
					fmt.Printf("[%v] - GettingItem: Done\n", id)
					item := parseHNItem(hnItem)
					if isStoryLink(item) {
						stories = append(stories, item)
					}
				}(itemID)
			}
			wg.Wait()
			fmt.Println("I am checking size: ", len(stories))
			if len(stories) >= *numStories {
				break
			}
		}
		sort.Slice(stories, func(i, j int) bool {
			return rankingMap[stories[i].ID] < rankingMap[stories[j].ID]
		})
		stories = stories[:*numStories]
		data := templateData{
			Stories: stories,
			Time:    time.Since(start),
		}
		ctx.HTML(http.StatusOK, "index.html", gin.H{
			"data": data,
		})
	}
}
func feedHandler(numStories *int) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		client := hn.New(HN_ENDPOINT)
		start := time.Now()
		ids, err := client.TopItems()

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, err)
		}

		var stories []item
		for _, id := range ids {
			hnItem, err := client.GetItem(id, false)
			if err != nil {
				continue
			}
			item := parseHNItem(hnItem)
			if isStoryLink(item) {
				stories = append(stories, item)
				if len(stories) >= *numStories {
					break
				}
			}
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

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	numStories := flag.Int("limit", 5, "number of hacker news stories to fetch")
	flag.Parse()

	// typical fetch
	r.GET("/feed", feedHandler(numStories))
	// fetch concurrently
	r.GET("/feedc", feedConcurrentHandler(numStories, false))
	// fetch concurrently with cache
	r.GET("/feedcc", feedConcurrentHandler(numStories, true))
	// check
	r.GET("/hi", hiHandler)
	r.Run()
}
