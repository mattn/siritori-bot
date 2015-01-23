package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/daviddengcn/go-colortext"
	"github.com/garyburd/go-oauth/oauth"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type option map[string]string

var upper = strings.NewReplacer(
	"ぁ", "あ",
	"ぃ", "い",
	"ぅ", "う",
	"ぇ", "え",
	"ぉ", "お",
	"ゃ", "や",
	"ゅ", "ゆ",
	"ょ", "よ",
)

func kana2hira(s string) string {
	return strings.Map(func(r rune) rune {
		if 0x30A1 <= r && r <= 0x30F6 {
			return r - 0x0060
		}
		return r
	}, s)
}

func hira2kana(s string) string {
	return strings.Map(func(r rune) rune {
		if 0x3041 <= r && r <= 0x3096 {
			return r + 0x0060
		}
		return r
	}, s)
}

func search(dict, text string) string {
	rs := []rune(text)
	r := rs[len(rs)-1]

	f, err := os.Open(dict)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := bufio.NewReader(f)

	words := []string{}
	for {
		b, _, err := buf.ReadLine()
		if err != nil {
			break
		}
		line := string(b)
		if ([]rune(line))[0] == r {
			words = append(words, line)
		}
	}
	if len(words) == 0 {
		return ""
	}
	return words[rand.Int()%len(words)]
}

func shiritori(dict, text string) string {
	text = strings.Replace(text, "ー", "", -1)
	if rand.Int()%2 == 0 {
		text = hira2kana(text)
	} else {
		text = kana2hira(text)
	}
	return search(dict, text)
}

func handleText(dict, text string) string {
	rs := []rune(strings.TrimSpace(text))
	if len(rs) == 0 {
		return "なんやねん"
	}
	if rs[len(rs)-1] == 'ん' || rs[len(rs)-1] == 'ン' {
		return "出直して来い"
	}
	s := shiritori(dict, text)
	if s == "" {
		return "わかりません"
	}
	rs = []rune(s)
	if rs[len(rs)-1] == 'ん' || rs[len(rs)-1] == 'ン' {
		s += "\nあっ..."
	}
	return s
}

type Tweet struct {
	Text       string `json:"text"`
	Identifier string `json:"id_str"`
	User       struct {
		ScreenName string `json:"screen_name"`
	} `json:"user"`
}

var oauthClient = oauth.Client{
	TemporaryCredentialRequestURI: "https://api.twitter.com/oauth/request_token",
	ResourceOwnerAuthorizationURI: "https://api.twitter.com/oauth/authenticate",
	TokenRequestURI:               "https://api.twitter.com/oauth/access_token",
}

func clientAuth(requestToken *oauth.Credentials) (*oauth.Credentials, error) {
	cmd := "xdg-open"
	url_ := oauthClient.AuthorizationURL(requestToken, nil)

	args := []string{cmd, url_}
	if runtime.GOOS == "windows" {
		cmd = "rundll32.exe"
		args = []string{cmd, "url.dll,FileProtocolHandler", url_}
	} else if runtime.GOOS == "darwin" {
		cmd = "open"
		args = []string{cmd, url_}
	} else if runtime.GOOS == "plan9" {
		cmd = "plumb"
	}
	ct.ChangeColor(ct.Red, true, ct.None, false)
	fmt.Println("Open this URL and enter PIN.", url_)
	ct.ResetColor()
	cmd, err := exec.LookPath(cmd)
	if err == nil {
		p, err := os.StartProcess(cmd, args, &os.ProcAttr{Dir: "", Files: []*os.File{nil, nil, os.Stderr}})
		if err != nil {
			log.Fatal("failed to start command:", err)
		}
		defer p.Release()
	}

	fmt.Print("PIN: ")
	stdin := bufio.NewReader(os.Stdin)
	b, err := stdin.ReadBytes('\n')
	if err != nil {
		log.Fatal("canceled")
	}

	if b[len(b)-2] == '\r' {
		b = b[0 : len(b)-2]
	} else {
		b = b[0 : len(b)-1]
	}
	accessToken, _, err := oauthClient.RequestToken(http.DefaultClient, requestToken, string(b))
	if err != nil {
		log.Fatal("failed to request token:", err)
	}
	return accessToken, nil
}

func getAccessToken(config map[string]string) (*oauth.Credentials, bool, error) {
	oauthClient.Credentials.Token = config["ClientToken"]
	oauthClient.Credentials.Secret = config["ClientSecret"]

	authorized := false
	var token *oauth.Credentials
	accessToken, foundToken := config["AccessToken"]
	accessSecert, foundSecret := config["AccessSecret"]
	if foundToken && foundSecret {
		token = &oauth.Credentials{accessToken, accessSecert}
	} else {
		requestToken, err := oauthClient.RequestTemporaryCredentials(http.DefaultClient, "", nil)
		if err != nil {
			log.Print("failed to request temporary credentials:", err)
			return nil, false, err
		}
		token, err = clientAuth(requestToken)
		if err != nil {
			log.Print("failed to request temporary credentials:", err)
			return nil, false, err
		}

		config["AccessToken"] = token.Token
		config["AccessSecret"] = token.Secret
		authorized = true
	}
	return token, authorized, nil
}

func getTweets(token *oauth.Credentials, url_ string, opt option) ([]Tweet, error) {
	param := make(url.Values)
	for k, v := range opt {
		param.Set(k, v)
	}
	oauthClient.SignParam(token, "GET", url_, param)
	url_ = url_ + "?" + param.Encode()
	res, err := http.Get(url_)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, err
	}
	var tweets []Tweet
	err = json.NewDecoder(res.Body).Decode(&tweets)
	if err != nil {
		return nil, err
	}
	return tweets, nil
}

func postTweet(token *oauth.Credentials, url_ string, opt option) error {
	param := make(url.Values)
	for k, v := range opt {
		param.Set(k, v)
	}
	oauthClient.SignParam(token, "POST", url_, param)
	res, err := http.PostForm(url_, url.Values(param))
	if err != nil {
		log.Println("failed to post tweet:", err)
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Println("failed to get timeline:", err)
		return err
	}
	return nil
}

func getConfig() (string, map[string]string) {
	home := os.Getenv("HOME")
	dir := filepath.Join(home, ".config")
	if runtime.GOOS == "windows" {
		home = os.Getenv("USERPROFILE")
		dir = os.Getenv("APPDATA")
		if dir == "" {
			dir = filepath.Join(home, "Application Data")
		}
	} else if runtime.GOOS == "plan9" {
		home = os.Getenv("home")
		dir = filepath.Join(home, ".config")
	}
	_, err := os.Stat(dir)
	if err != nil {
		if os.Mkdir(dir, 0700) != nil {
			log.Fatal("failed to create directory:", err)
		}
	}
	dir = filepath.Join(dir, "siritori-bot")
	_, err = os.Stat(dir)
	if err != nil {
		if os.Mkdir(dir, 0700) != nil {
			log.Fatal("failed to create directory:", err)
		}
	}
	file := filepath.Join(dir, "settings.json")
	config := map[string]string{}

	b, err := ioutil.ReadFile(file)
	if err != nil {
		config["ClientToken"] = "u4jCd2cRaZXhzmbGOz8gMQ"
		config["ClientSecret"] = "mGpgKcRHFw8slfexvFfCtmorSJPygdKLGZRFytihY"
	} else {
		err = json.Unmarshal(b, &config)
		if err != nil {
			log.Fatal("could not unmarhal settings.json:", err)
		}
	}
	return file, config
}

func main() {
	file, config := getConfig()

	// setup twitter authorization
	token, authorized, err := getAccessToken(config)
	if err != nil {
		log.Fatal("faild to get access token:", err)
	}
	if authorized {
		b, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			log.Fatal("failed to store file:", err)
		}
		err = ioutil.WriteFile(file, b, 0700)
		if err != nil {
			log.Fatal("failed to store file:", err)
		}
	}

	// look dictionary file
	var dict string
	if s, ok := config["Dictionary"]; ok {
		dict = s
	} else {
		dict = filepath.Join(filepath.Dir(os.Args[0]), "dict.txt")
	}

	var since string

	for {
		time.Sleep(30 * time.Second)
		opt := option{}
		if since != "" {
			opt["since_id"] = since
		}
		log.Println("polling... ", since)
		tweets, err := getTweets(token, "https://api.twitter.com/1.1/statuses/mentions_timeline.json", opt)
		if err != nil {
			log.Println(err)
			continue
		}
		if len(tweets) == 0 {
			log.Println("no mentions")
			continue
		}
		if since != "" {
			for _, tweet := range tweets {
				status := fmt.Sprintf("@%s %s", tweet.User.ScreenName, handleText(dict, tweet.Text))
				log.Println(status)
				err = postTweet(token, "https://api.twitter.com/1.1/statuses/update.json", option{"status": status, "in_reply_to_status_id": tweet.Identifier})
				if err != nil {
					log.Println(err)
				}
				time.Sleep(5 * time.Second)
			}
		}
		since = tweets[0].Identifier
	}
}
