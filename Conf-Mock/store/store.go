package store

import (
	"fmt"
	"sync"
	"time"
)

type Page struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Status  string                 `json:"status"`
	Title   string                 `json:"title"`
	Space   map[string]string      `json:"space"`
	Body    map[string]interface{} `json:"body"`
	Version map[string]interface{} `json:"version"`
	Links   map[string]string      `json:"_links"`
}

var (
	mu      sync.RWMutex
	pages   = make(map[string]*Page)
	nextID  = 10001
	baseURL = "http://localhost:8090"
)

func SetBaseURL(url string) {
	baseURL = url
}

func Create(title, spaceKey, bodyHTML string) *Page {
	mu.Lock()
	defer mu.Unlock()

	id := fmt.Sprintf("%d", nextID)
	nextID++

	p := &Page{
		ID:     id,
		Type:   "page",
		Status: "current",
		Title:  title,
		Space:  map[string]string{"key": spaceKey},
		Body: map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          bodyHTML,
				"representation": "storage",
			},
		},
		Version: map[string]interface{}{
			"number":    1,
			"when":      time.Now().UTC().Format(time.RFC3339),
			"minorEdit": false,
		},
		Links: map[string]string{
			"self":  fmt.Sprintf("%s/rest/api/content/%s", baseURL, id),
			"webui": fmt.Sprintf("/pages/viewpage.action?pageId=%s", id),
		},
	}
	pages[id] = p
	return p
}

func Get(id string) *Page {
	mu.RLock()
	defer mu.RUnlock()
	return pages[id]
}

func Update(id, title, bodyHTML string, versionNumber int) (*Page, error) {
	mu.Lock()
	defer mu.Unlock()

	p, ok := pages[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	currentVer, _ := p.Version["number"].(int)
	if versionNumber != currentVer+1 {
		return nil, fmt.Errorf("version conflict: expected %d, got %d", currentVer+1, versionNumber)
	}

	if title != "" {
		p.Title = title
	}
	if bodyHTML != "" {
		p.Body = map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          bodyHTML,
				"representation": "storage",
			},
		}
	}
	p.Version = map[string]interface{}{
		"number":    versionNumber,
		"when":      time.Now().UTC().Format(time.RFC3339),
		"minorEdit": false,
	}
	return p, nil
}

func Delete(id string) bool {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := pages[id]; !ok {
		return false
	}
	delete(pages, id)
	return true
}

func Search(spaceKey, title string, start, limit int) ([]*Page, int) {
	mu.RLock()
	defer mu.RUnlock()

	var results []*Page
	for _, p := range pages {
		if spaceKey != "" && p.Space["key"] != spaceKey {
			continue
		}
		if title != "" && p.Title != title {
			continue
		}
		results = append(results, p)
	}

	total := len(results)
	if start >= total {
		return []*Page{}, total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return results[start:end], total
}
