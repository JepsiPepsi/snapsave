package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	FilesExisted   int
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
	UnixTime      int64
}

const (
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"
	baseURL   = "https://story.snapchat.com/@"
)

func main() {
	getUserInput()
}

func getUserInput() {
	// Define flags
	filePath := flag.String("file", "", "Path to the file containing usernames.")
	intervalFlag := flag.Int("interval", 0, "Interval for scraping in hours.")
	outputDir := flag.String("output", ".", "Output directory for scraped data.")
	user := flag.String("user", "", "Specify a user to scrape, comma-separated for multiple users.")
	help := flag.Bool("help", false, "Show help message")

	// Parse flags
	flag.Parse()

	// Show help message if --help is provided
	if *help {
		flag.Usage()
		return
	}

	// Initialize the config struct
	config := &Config{}

	// Attempt to load environment variables if flags are not set
	if *filePath == "" {
		*filePath = os.Getenv("USER_FILE")
	}
	if *intervalFlag == 0 {
		if intervalEnv, ok := os.LookupEnv("INTERVAL"); ok {
			if interval, err := strconv.Atoi(intervalEnv); err == nil {
				*intervalFlag = interval
			}
		}
	}
	if *outputDir == "." {
		if dirEnv := os.Getenv("DOWNLOAD_DIR"); dirEnv != "" {
			*outputDir = dirEnv
		}
	}
	if *user == "" {
		*user = os.Getenv("SNAP_USERS")
	}

	// Setup config based on flags or environment variables
	config.Interval = *intervalFlag
	config.Directory = *outputDir
	config.UserCount = 1

	// Error handling for mutually exclusive flags
	if *filePath != "" && *user != "" {
		fmt.Println("Error: --file and --user flags are mutually exclusive. Please specify only one.")
		flag.Usage()
		os.Exit(1)
	}

	// Processing --user flag
	if *user != "" {
		users := strings.Split(*user, ",")
		config.Users = make(map[string]User)
		for _, u := range users {
			config.Users[u] = User{Username: u}
		}
		setupUserDirectory(config)
		startScraper(config)
	}

	// Processing --file flag
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
		fmt.Printf("Starting interval scraping, next scrape in %v minute(s)\n", *intervalFlag)
		timer := time.NewTicker(time.Duration(*intervalFlag) * time.Minute)
		for range timer.C {
			startScraper(config)
			fmt.Printf("Starting interval scraping, next scrape in %v minute(s)\n", *intervalFlag)
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
	}

	log.Info("Scraping complete\n")
	for u := range config.Users {
		fmt.Printf("User %s has %v new downloads. %v already existed.\n", u, config.Users[u].NewDownloads, config.Users[u].FilesExisted)
	}

	for u := range config.Users {
		user := config.Users[u]
		user.TotalDownloads = 0
		user.NewDownloads = 0
		user.FilesExisted = 0
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
		log.Infof("User %s has no stories\n", u)
	}

	if !spotlightHighlights.Exists() && spotlightHighlights.String() == "" {
		log.Infof("User %s has no spotlight highlights\n", u)
	}

	if !spotlightStoryMetadata.Exists() && spotlightStoryMetadata.String() == "" {
		log.Infof("User %s has no spotlight story metadata\n", u)
	}

	if !curatedHighlights.Exists() && curatedHighlights.String() == "" {
		log.Infof("User %s has no curated highlights\n", u)
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
		snap.SnapID = value.Get("videoMetadata.uploadDateMs").String()

		if snap.SnapID == "" {
			snap.SnapID = time.RFC3339Nano
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
		snap.UnixTime = value.Get("timestampInSec.value").Int()

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
		progressbar.OptionOnCompletion(func() {
			fmt.Printf("\n\n")
		}))
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

	if s.SnapType == "story" {
		timestamp := time.Unix(s.UnixTime, 0)

		dateStr := timestamp.Format("02-01-2006")

		dir = filepath.Join(dir, dateStr)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Errorf("Failed to create directory %s: %v\n", dir, err)
		return
	}

	path := filepath.Join(dir, fmt.Sprintf("%s-%s%s", s.SnapID, s.Index, s.FileExt))

	if _, err := os.Stat(path); err == nil {
		user := config.Users[u]
		user.FilesExisted++
		config.Users[u] = user
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
		return err
	}

	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
