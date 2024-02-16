package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/log"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"
)

type User struct {
	Username       string
	TotalDownloads int
	NewDownloads   int
	Data           string
}

type Config struct {
	Interval  int
	FilePath  string
	Directory string
	Users     (map[string]User)
	UserCount int
}

type Snap struct {
	SnapType      string
	SnapID        string
	MediaURL      string
	Index         string
	SnapMediaType int64
	Username      string
	Directory     string
	FileExt       string
}

const (
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"
	baseURL   = "https://story.snapchat.com/@"
)

func main() {
	getUserInput()
}

func getUserInput() {
	filePath := flag.String("file", "", "Path to the file containing usernames. ENV variable USER_FILE")
	intervalFlag := flag.Int("interval", 0, "Interval for scraping in hours. ENV variable: INTERVAL")
	outputDir := flag.String("output", ".", "Output directory for scraped data. ENV variable: DOWNLOAD_DIR")
	user := flag.String("user", "", "Specify a user to scrape, comma-separated for multiple users.")

	help := flag.Bool("help", false, "Show help message")

	flag.Parse()

	// Show help message
	if *help {
		flag.Usage()
		return
	}

	// Initialize the config struct
	config := &Config{}

	// Default config setup
	config.Interval = *intervalFlag
	config.Directory = *outputDir
	config.UserCount = 1

	// Error if both --file and --user are used
	if *filePath != "" && *user != "" {
		fmt.Println("Error: --file and --user flags are mutually exclusive. Please specify only one.")
		flag.Usage()
		os.Exit(1)
	}

	// Handling --user flag
	if *user != "" {
		users := strings.Split(*user, ",")
		config.Users = make(map[string]User)
		for _, u := range users {
			config.Users[u] = User{Username: u}
		}
		setupUserDirectory(config)
		startScraper(config)
	}

	// Handling --file flag
	if *filePath != "" {
		config.FilePath = *filePath
		if _, err := getUsersFromFile(config); err != nil {
			fmt.Printf("Failed to read users from file: %v\n", err)
			return
		}
		setupUserDirectory(config)
		startScraper(config)
	}

	// Interval handling
	if *intervalFlag != 0 {
		fmt.Printf("Starting interval scraping, next scrape in %v minutes(s)\n", *intervalFlag)
		timer := time.NewTicker(time.Duration(*intervalFlag) * time.Minute)
		for range timer.C {
			startScraper(config)
			fmt.Printf("Starting interval scraping, next scrape in %v minutes(s)\n", *intervalFlag)
		}
	}
}

func startScraper(config *Config) {

	timer := time.Now()
	log.Info("Starting scraper\n")

	for u := range config.Users {
		bar := newBar(config, config.Users[u].Username)

		data, err := fetchJSON(baseURL + config.Users[u].Username)
		if err != nil {
			log.Errorf("Failed to fetch data for user %s: %v\n", u, err)
			continue
		}
		scrapeData(data, config, config.Users[u].Username, bar)
		config.UserCount++
		bar.Reset()

	}

	log.Info("Scraping complete\n")
	for u := range config.Users {
		fmt.Printf("User %s has %v new downloads\n", u, config.Users[u].NewDownloads)
	}

	for u := range config.Users {
		user := config.Users[u]
		user.TotalDownloads = 0
		user.NewDownloads = 0
		config.Users[u] = user
	}

	config.UserCount = 1
	fmt.Println("Total time: ", time.Since(timer))
}

func scrapeData(s string, config *Config, u string, b *progressbar.ProgressBar) {

	snap := Snap{
		Username:  u,
		Directory: config.Directory,
	}

	// Extract the snapList, spotlightHighlights, spotlightStoryMetadata, and curatedHighlights fields
	snapList := gjson.Get(s, "props.pageProps.story.snapList")
	spotlightHighlights := gjson.Get(s, "props.pageProps.spotlightHighlights")
	spotlightStoryMetadata := gjson.Get(s, "props.pageProps.spotlightStoryMetadata")
	curatedHighlights := gjson.Get(s, "props.pageProps.curatedHighlights")

	//Check if the fields are populated
	if !snapList.Exists() && snapList.String() == "" {
		fmt.Println("No snaps found for user")
	}

	if !spotlightHighlights.Exists() && spotlightHighlights.String() == "" {
		fmt.Println("User has no spotlight highlights")
	}

	if !spotlightStoryMetadata.Exists() && spotlightStoryMetadata.String() == "" {
		fmt.Println("User has no spotlight story metadata")
	}

	if !curatedHighlights.Exists() && curatedHighlights.String() == "" {
		fmt.Println("User has no curated highlights")
	}

	totalDownloads := 0

	// Get the total number of snaps
	curatedHighlights.ForEach(func(key, value gjson.Result) bool {
		totalDownloads += int(value.Get("snapList.#").Int())
		return true
	})

	spotlightHighlights.ForEach(func(key, value gjson.Result) bool {
		totalDownloads += int(value.Get("snapList.#").Int())
		return true
	})

	spotlightStoryMetadata.ForEach(func(key, value gjson.Result) bool {
		totalDownloads++
		return true
	})

	snapList.ForEach(func(key, value gjson.Result) bool {
		totalDownloads++
		return true
	})

	// Update the TotalDownloads for the user in the Config.Users map
	if user, exists := config.Users[u]; exists {
		user.TotalDownloads += totalDownloads
		config.Users[u] = user
	} else {
		fmt.Printf("User %s not found in config.Users\n", u)
	}

	b.ChangeMax(config.Users[u].TotalDownloads)

	curatedHighlights.ForEach(func(key, value gjson.Result) bool {

		curatedHighlightsSnapList := value.Get("snapList")
		snap.SnapID = value.Get("highlightId.value").String()

		if snap.SnapID == "" {
			snap.SnapID = value.Get("storyId.value").String()
			if snap.SnapID == "" {
				snap.SnapID = time.RFC3339Nano
			}
		}

		curatedHighlightsSnapList.ForEach(func(key, value gjson.Result) bool {

			snap.SnapType = "curatedHighlights"
			snap.MediaURL = value.Get("snapUrls.mediaUrl").String()
			snap.SnapMediaType = value.Get("snapMediaType").Int()
			snap.Index = value.Get("snapIndex").String()

			processSnap(snap, config, u)

			b.Add(1)

			return true
		})
		return true
	})

	spotlightStoryMetadata.ForEach(func(key, value gjson.Result) bool {

		snap.SnapType = "spotlightStory"
		snapId := value.Get("videoMetadata.uploadDateMs").String()

		if snapId == "" {
			snapId = time.RFC3339Nano
		}

		snap.MediaURL = value.Get("videoMetadata.contentUrl").String()
		snap.SnapMediaType = 1

		processSnap(snap, config, u)

		b.Add(1)

		return true
	})

	spotlightHighlights.ForEach(func(key, value gjson.Result) bool {
		spotlightSnaplist := value.Get("snapList")
		spotlightSnaplist.ForEach(func(key, value gjson.Result) bool {

			snap.SnapType = "spotlightHighlights"
			snap.SnapID = value.Get("snapId.value").String()

			if snap.SnapID == "" {
				snap.SnapID = time.RFC3339Nano
			}

			snap.SnapMediaType = value.Get("snapMediaType").Int()
			snap.MediaURL = value.Get("snapUrls.mediaUrl").String()
			snap.Index = value.Get("snapIndex").String()

			processSnap(snap, config, u)

			b.Add(1)

			return true
		})
		return true
	})

	snapList.ForEach(func(key, value gjson.Result) bool {

		snap.SnapType = "story"
		snap.SnapID = value.Get("snapId.value").String()

		if snap.SnapID == "" {
			snap.SnapID = time.RFC3339Nano
		}

		snap.SnapMediaType = value.Get("snapMediaType").Int()
		snap.MediaURL = value.Get("snapUrls.mediaUrl").String()

		processSnap(snap, config, u)

		b.Add(1)

		return true
	})

}

func getUsersFromFile(config *Config) (*Config, error) {
	file, err := os.Open(config.FilePath)
	if err != nil {
		return config, err
	}
	defer file.Close()

	if config.Users == nil {
		config.Users = make(map[string]User)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		username := scanner.Text()
		config.Users[username] = User{Username: username}
	}
	return config, scanner.Err()
}

func setupUserDirectory(config *Config) error {
	paths := []string{"story", "spotlightHighlights", "spotlightStory", "curatedHighlights"}

	for _, p := range paths {
		for _, u := range config.Users {
			path := filepath.Join(config.Directory, u.Username)
			if _, err := os.Stat(filepath.Join(path, p)); os.IsNotExist(err) {
				err := os.MkdirAll(filepath.Join(path, p), 0755)
				if err != nil {
					log.Error("Failed to create directory for user %s: %v\n", u.Username, err)
					return err
				}
			}
		}
	}
	return nil
}

func fetchJSON(url string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	data := doc.Find("#__NEXT_DATA__").Text()

	return data, nil
}

func newBar(config *Config, u string) *progressbar.ProgressBar {
	bar := progressbar.NewOptions(config.Users[u].TotalDownloads,
		progressbar.OptionShowCount(),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(false),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetDescription(fmt.Sprintf("[cyan][%v/%v][reset] Scraping... %v", config.UserCount, len(config.Users), u)),
	)
	return bar
}

func processSnap(s Snap, config *Config, u string) {
	switch s.SnapMediaType {
	case 0:
		s.FileExt = ".png"
	case 1:
		s.FileExt = ".mp4"
	default:
		log.Errorf("Unknown media type: %v", s.SnapMediaType)
		s.FileExt = ".unknown"
	}

	dir := filepath.Join(s.Directory, s.Username, s.SnapType)
	path := filepath.Join(dir, fmt.Sprintf("%s-%s%s", s.SnapID, s.Index, s.FileExt))

	if _, err := os.Stat(path); err == nil {
		return
	} else if !os.IsNotExist(err) {
		fmt.Printf("Failed to check if file exists: %v\n", err)
		return
	}

	err := downloadFile(s.MediaURL, path)
	if err != nil {
		fmt.Printf("Failed to download snap: %v\n", err)
	} else {
		user := config.Users[u]
		user.NewDownloads++
		config.Users[u] = user
	}
}

func downloadFile(u, p string) error {
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(p)
	if err != nil {
		fmt.Println("Failed to create file")
		return err
	}

	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
