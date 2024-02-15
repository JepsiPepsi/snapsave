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
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/log"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"
)

const (
	baseURL   = "https://story.snapchat.com/@"
	userAgent = "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:94.0) Gecko/20100101 Firefox/103.0.2"
)

var (
	users            int
	currentUser      int
	totalDownloads   int
	downloadCount    int
	newDownloadCount int
)

func main() {

	log.Infof("Starting Scraper...\n")

	filePath := flag.String("userfile", "users.txt", "Path to the file containing usernames. ENV variable USER_FILE")
	intervalFlag := flag.Int("interval", 0, "Interval for scraping in hours. ENV variable: INTERVAL")
	outputDir := flag.String("output", ".", "Output directory for scraped data. ENV variable: DOWNLOAD_DIR")
	help := flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	if flag.NArg() > 0 {
		user := flag.Arg(0)
		log.Infof("Single user mode: %s\n", user)
		getSingleUser(user, *outputDir)
		return
	}

	if *outputDir == "." {
		if envDir, exists := os.LookupEnv("DL_DIR"); exists {
			*outputDir = envDir
		}
	}

	if *filePath == "users.txt" {
		if envFile, exists := os.LookupEnv("FILE"); exists {
			*filePath = envFile
		}
	}

	intVal := *intervalFlag
	if intVal == 0 {
		envInterval, err := strconv.Atoi(os.Getenv("INTERVAL"))
		if err == nil {
			intVal = envInterval
		} else {
			runProcess(*filePath, *outputDir, &users, &currentUser)
			return
		}
	}

	log.Infof("Using file: %s\n", *filePath)
	log.Infof("Interval set to: %d hours\n", intVal)
	log.Infof("Output directory set to: %s\n", *outputDir)

	interval := time.Duration(intVal) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runProcess(*filePath, *outputDir, &users, &currentUser)

	for range ticker.C {
		log.Infof("Running process at: %s\n", time.Now().Add(interval).Format(time.RFC1123))
		runProcess(*filePath, *outputDir, &users, &currentUser)
	}
}

func getSingleUser(username string, baseDir string) {

	log.Infof("Processing user: %s\n", username)
	userDir := setupUserDirectory(baseDir, username)
	if userDir == "" {
		log.Error("Skipping user due to directory setup failure.")
	}

	log.Infof("Fetching JSON data for user %s\n", username)
	jsonData, err := getJSON(baseURL + username)
	if err != nil {
		log.Errorf("Failed to get JSON data for %s: %v\n", username, err)
	}

	log.Infof("Downloading snap list for user %s\n", username)
	scrapeData(jsonData, userDir)
	log.Infof(" Completed processing for %s\n", username)
}

func runProcess(usernamesFilePath, baseDir string, c *int, u *int) {
	log.Infof("Reading usernames from file %v", usernamesFilePath)
	usernames, int, err := readUsernamesFromFile(usernamesFilePath)
	if err != nil {
		log.Errorf("Failed to read usernames from file: %v\n", err)
		return
	}

	*c = int

	for _, username := range usernames {
		totalDownloads = 0
		log.Infof("Processing user: %s\n", username)
		userDir := setupUserDirectory(baseDir, username)
		if userDir == "" {
			log.Errorf("Skipping user due to directory setup failure.")
			continue
		}

		log.Infof("Fetching JSON data for user %s\n", username)
		jsonData, err := getJSON(baseURL + username)
		if err != nil {
			log.Errorf("Failed to get JSON data for %s: %v\n", username, err)
			continue
		}
		*u++
		log.Infof("Downloading snap list for user %s\n", username)
		scrapeData(jsonData, userDir)
	}

	fmt.Println("Completed processing for all users.")
}

func readUsernamesFromFile(filePath string) ([]string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	var usernames []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		usernames = append(usernames, scanner.Text())
	}

	n := len(usernames)

	return usernames, n, scanner.Err()
}

func setupUserDirectory(baseDir, username string) string {
	path := filepath.Join(baseDir, username)
	storyPath := fmt.Sprintf("%s/story", path)
	spotlightHighlightsPath := fmt.Sprintf("%s/spotlightHighlights", path)
	spotlightStoryPath := fmt.Sprintf("%s/spotlightStory", path)
	curatedHighlightsPath := fmt.Sprintf("%s/curatedHighlights", path)

	if _, err := os.Stat(curatedHighlightsPath); os.IsNotExist(err) {
		err := os.MkdirAll(curatedHighlightsPath, 0755)
		if err != nil {
			log.Error("Failed to create directory for user %s: %v\n", username, err)
			return ""
		}
	}

	if _, err := os.Stat(storyPath); os.IsNotExist(err) {
		err := os.MkdirAll(storyPath, 0755)
		if err != nil {
			log.Error("Failed to create directory for user %s: %v\n", username, err)
			return ""
		}
	}

	if _, err := os.Stat(spotlightHighlightsPath); os.IsNotExist(err) {
		err := os.MkdirAll(spotlightHighlightsPath, 0755)
		if err != nil {
			log.Error("Failed to create directory for user %s: %v\n", username, err)
			return ""
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			log.Error("Failed to create directory for user %s: %v\n", username, err)
			return ""
		}
	}

	if _, err := os.Stat(spotlightStoryPath); os.IsNotExist(err) {
		err := os.MkdirAll(spotlightStoryPath, 0755)
		if err != nil {
			log.Error("Failed to create directory for user %s: %v\n", username, err)
			return ""
		}
	}

	return path
}

func getJSON(url string) (string, error) {
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
		return "", fmt.Errorf("no connection with Snapchat")
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	data := doc.Find("#__NEXT_DATA__").Text()

	return data, nil
}

func downloadFile(url, filePath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	path := filePath

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func scrapeData(s string, userDir string) {

	snapList := gjson.Get(s, "props.pageProps.story.snapList")
	spotlightHighlights := gjson.Get(s, "props.pageProps.spotlightHighlights")
	spotlightStoryMetadata := gjson.Get(s, "props.pageProps.spotlightStoryMetadata")
	curatedHighlights := gjson.Get(s, "props.pageProps.curatedHighlights")

	if !snapList.Exists() || snapList.String() == "" {
		fmt.Println("User has no Snap-stories")
	}

	if !spotlightHighlights.Exists() || spotlightHighlights.String() == "" {
		fmt.Println("User has no spotlight highlights")
	}

	if !spotlightStoryMetadata.Exists() || spotlightStoryMetadata.String() == "" {
		fmt.Println("User has no spotlight stories")
	}

	if !curatedHighlights.Exists() || curatedHighlights.String() == "" {
		fmt.Println("User has no curated highlights")
	}

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

	bar := newBar(totalDownloads, &users, &currentUser)

	curatedHighlights.ForEach(func(key, value gjson.Result) bool {

		curatedHighlightsSnapList := value.Get("snapList")
		snapId := value.Get("highlightId.value").String()

		if snapId == "" {
			snapId = value.Get("storyId.value").String()
			if snapId == "" {
				snapId = time.RFC3339Nano
			}
		}

		curatedHighlightsSnapList.ForEach(func(key, value gjson.Result) bool {
			snapType := "curatedHighlights"
			mediaUrl := value.Get("snapUrls.mediaUrl").String()
			snapMediaType := value.Get("snapMediaType").Int()
			index := value.Get("snapIndex").String()

			processSnap(snapType, snapId, mediaUrl, index, userDir, int(snapMediaType), &downloadCount, totalDownloads, &newDownloadCount)
			downloadCount++
			bar.Add(1)
			if downloadCount == totalDownloads {
				fmt.Printf(" Downloading completed. New downloads: %d\n", newDownloadCount)
			}
			return true
		})
		return true
	})

	spotlightStoryMetadata.ForEach(func(key, value gjson.Result) bool {

		snapType := "spotlightStory"
		snapId := value.Get("videoMetadata.uploadDateMs").String()
		if snapId == "" {
			snapId = time.RFC3339Nano
		}
		mediaUrl := value.Get("videoMetadata.contentUrl").String()
		snapMediaType := 1

		processSnap(snapType, snapId, mediaUrl, "", userDir, snapMediaType, &downloadCount, totalDownloads, &newDownloadCount)
		downloadCount++
		bar.Add(1)
		if downloadCount == totalDownloads {
			fmt.Printf("Downloading completed. New downloads: %d\n", newDownloadCount)
		}
		return true
	})

	spotlightHighlights.ForEach(func(key, value gjson.Result) bool {

		spotlightSnaplist := value.Get("snapList")
		spotlightSnaplist.ForEach(func(key, value gjson.Result) bool {
			snapType := "spotlightHighlights"
			snapId := value.Get("snapId.value").String()
			if snapId == "" {
				snapId = time.RFC3339Nano
			}
			snapMediaType := value.Get("snapMediaType").Int()
			mediaUrl := value.Get("snapUrls.mediaUrl").String()
			index := value.Get("snapIndex").String()

			processSnap(snapType, snapId, mediaUrl, index, userDir, int(snapMediaType), &downloadCount, totalDownloads, &newDownloadCount)
			downloadCount++
			bar.Add(1)
			if downloadCount == totalDownloads {
				fmt.Printf("Downloading completed. New downloads: %d\n", newDownloadCount)
			}
			return true
		})
		return true
	})

	snapList.ForEach(func(key, value gjson.Result) bool {

		snapType := "story"
		snapId := value.Get("snapId.value").String()
		if snapId == "" {
			snapId = time.RFC3339Nano
		}
		snapMediaType := value.Get("snapMediaType").Int()
		mediaUrl := value.Get("snapUrls.mediaUrl").String()

		processSnap(snapType, snapId, mediaUrl, "", userDir, int(snapMediaType), &downloadCount, totalDownloads, &newDownloadCount)
		downloadCount++
		bar.Add(1)
		if downloadCount == totalDownloads {
			fmt.Printf(" Downloading completed. New downloads: %d\n", newDownloadCount)
		}
		return true
	})
}

func processSnap(snapType, snapId, mediaUrl, index, userDir string, snapMediaType int, downloadedCount *int, totalDownloads int, newDownloadCount *int) {

	var fileExt string

	switch snapMediaType {
	case 0:
		fileExt = ".png"
	case 1:
		fileExt = ".mp4"
	default:
		log.Errorf("Unknown media type: %v", snapMediaType)
		fileExt = ".unknown"
	}

	path := filepath.Join(userDir, snapType)
	fileName := filepath.Join(path, snapId+index+fileExt)
	if _, err := os.Stat(fileName); err == nil {
		return
	} else if !os.IsNotExist(err) {
		fmt.Printf("Failed to check if file exists: %v\n", err)
		return
	}

	err := downloadFile(mediaUrl, fileName)
	if err != nil {
		fmt.Printf("Failed to download snap: %v\n", err)
	} else {
		(*newDownloadCount)++
	}
}

func newBar(s int, u *int, c *int) *progressbar.ProgressBar {

	fmt.Println(s)

	bar := progressbar.NewOptions(s,
		progressbar.OptionShowCount(),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetDescription(fmt.Sprintf("[cyan][%v/%v][reset] Scraping...", *c, *u)),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	return bar
}
