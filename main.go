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

type Mode int

const (
	Vanilla Mode = iota
	Concurrent
	ConcurrentWithCache
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
func feedHandler(mode Mode, numStories *int) gin.HandlerFunc {
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

func fetchConcurrent(config *feedConfigs) ([]item, error) {
	rankingMap := map[int]int{}
	for i, id := range config.IDs {
		rankingMap[id] = i
	}
	var stories []item
	batchSize := BATCH_SIZE
	for i := 0; i < len(config.IDs); i += batchSize {
		j := i + batchSize
		if j > len(config.IDs) {
			j = len(config.IDs)
		}
		var wg sync.WaitGroup
		batchIds := config.IDs[i:j]
		wg.Add(len(batchIds))
		for _, itemID := range batchIds {
			go func(id int) {
				defer wg.Done()
				fmt.Printf("[%v] - GettingItem: Start\n", id)
				hnItem, err := config.Client.GetItem(id, config.Mode == Concurrent)
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
		if len(stories) >= config.NumStories {
			break
		}
	}
	sort.Slice(stories, func(i, j int) bool {
		return rankingMap[stories[i].ID] < rankingMap[stories[j].ID]
	})
	stories = stories[:config.NumStories]
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
	if config.Mode == Vanilla {
		return fetchVanilla(config)
	}
	return fetchConcurrent(config)
}

type feedConfigs struct {
	Mode       Mode
	NumStories int
	IDs        []int
	Client     *hn.Client
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	numStories := flag.Int("limit", 5, "number of hacker news stories to fetch")
	flag.Parse()

	// typical fetch
	r.GET("/feed", feedHandler(Vanilla, numStories))
	// fetch concurrently
	r.GET("/feedc", feedHandler(Concurrent, numStories))
	// fetch concurrently with cache
	r.GET("/feedcc", feedHandler(ConcurrentWithCache, numStories))
	// check
	r.GET("/hi", hiHandler)
	r.Run()
}
