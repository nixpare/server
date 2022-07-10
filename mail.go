package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type GmailService struct {
	gmailService *gmail.Service
}

func (svc GmailService) SendEmail(user, from, to, subject, content string, files []string) error {

	var message gmail.Message

	boundary := RandStr(32, "alphanum")

	textBody := "Content-Type: multipart/mixed; boundary=" + boundary + " \n" +
		"MIME-Version: 1.0\n" +
		"To: " + to + "\n"
	
	if from != "" {
		textBody += "From: " + from + "\n"
	}

	textBody += "Subject: " + subject + "\n\n" +
		"--" + boundary + "\n" +
		"Content-Type: text/plain; charset=" + string('"') + "UTF-8" + string('"') + "\n" +
		"MIME-Version: 1.0\n" +
		"Content-Transfer-Encoding: 7bit\n\n" +
		content + "\n\n" +
		"--" + boundary

	for _, x := range files {
		fileBytes, err := ioutil.ReadFile(x)
		if err != nil {
			return err
		}
	
		fileMIMEType := http.DetectContentType(fileBytes)
	
		fileData := base64.StdEncoding.EncodeToString(fileBytes)

		fileParsed := strings.Split(x, "/")
		fileName := fileParsed[len(fileParsed)-1]

		textBody += "\n" +
			"Content-Type: " + fileMIMEType + "; name=" + string('"') + fileName + string('"') + " \n" +
			"MIME-Version: 1.0\n" +
			"Content-Transfer-Encoding: base64\n" +
			"Content-Disposition: attachment; filename=" + string('"') + fileName + string('"') + " \n\n" +
			chunkSplit(fileData, 76, "\n") +
			"--" + boundary
	}

	textBody += "--"

	messageBody := []byte(textBody)

	message.Raw = base64.URLEncoding.EncodeToString(messageBody)

	_, err := svc.gmailService.Users.Messages.Send(user, &message).Do()

	return err
}

func connectGmailService(credentials, token string) (*gmail.Service, error) {

	b, err := ioutil.ReadFile(credentials)
	if err != nil {
		return nil, err
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailModifyScope, gmail.GmailSettingsSharingScope)
	if err != nil {
		return nil, err
	}

	client, err := getClient(config, token)
	if err != nil {
		return nil, err
	}

	return gmail.NewService(context.Background(), option.WithHTTPClient(client))
}

func getClient(config *oauth2.Config, token string) (*http.Client, error) {

	tok, err := tokenFromFile(token)
	if err != nil {

		tok, err = getTokenFromWeb(config)
		if err != nil {
			return nil, err
		}

		err = saveToken(token, tok)
		if err != nil {
			return nil, err
		}
	}
	return config.Client(context.Background(), tok), nil
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, err
	}

	return config.Exchange(context.TODO(), authCode)
}

func tokenFromFile(file string) (*oauth2.Token, error) {

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) error {

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

// RandStr function
//   - strSize:  length of the output
//   - randType: type of output (alphanum | alpha | alphalow | num)
func RandStr(strSize int, randType string) string {

	var dictionary string

	switch randType {
	case "alphanum":
		dictionary = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	case "alpha":
		dictionary = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	case "alphalow":
		dictionary = "abcdefghijklmnopqrstuvwxyz"
	case "num":
		dictionary = "0123456789"
	default:
		return ""
	}

	var strBytes = make([]byte, strSize)
	_, _ = rand.Read(strBytes)
	for k, v := range strBytes {
		strBytes[k] = dictionary[v%byte(len(dictionary))]
	}
	return string(strBytes)

}

func chunkSplit(body string, limit int, end string) string {

	var charSlice []rune

	for _, char := range body {
		charSlice = append(charSlice, char)
	}

	var result = ""

	for len(charSlice) >= 1 {
	
		result = result + string(charSlice[:limit]) + end

		charSlice = charSlice[limit:]

		if len(charSlice) < limit {
			limit = len(charSlice)
		}
	}
	return result

}
