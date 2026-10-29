package feed

import (
	"fmt"
	"github.com/mmcdole/gofeed"
	"io"
	"net/http"
)

type Feed struct {
	url, name string
}

func Fetch(feed Feed) (*gofeed.Feed, error) {
	res, err := http.Get(feed.url)

	if err != nil {
		fmt.Println("Request error:", err)
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("Read body failed:", err)
		return nil, err
	}

	fp := gofeed.NewParser()
	content := string(body)

	result, err := fp.ParseString(content)
	if err != nil {
		fmt.Println("Could not parse feed:", err)
		return nil, err
	}

	return result, nil
}
