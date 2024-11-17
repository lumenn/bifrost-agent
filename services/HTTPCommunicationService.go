package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func GetRequestBody(url string) (string, error) {
	resp, err := http.Get(url)

	errorMessage := fmt.Errorf("could not get content")
	if err != nil {
		return "", errorMessage
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errorMessage
	}

	return string(body), nil
}

func PostForm(url string, values url.Values) (string, error) {
	resp, err := http.PostForm(url, values)

	if err != nil {
		return "", fmt.Errorf("failed to send form data")
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response from the server")
	}

	return string(responseBody), nil
}

func PostJSON(url string, body interface{}) (string, error) {
	errorMessage := fmt.Errorf("could not get content")
	println("Preinner: ", fmt.Sprintf("%v", body))

	jsonData, err := json.Marshal(body)
	if err != nil {
		return "", errorMessage
	}

	println("Inner: ", string(jsonData))

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", errorMessage
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errorMessage
	}

	return string(responseBody), nil
}
