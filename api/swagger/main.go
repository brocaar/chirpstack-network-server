// simple tool to merge different swagger definition into a single file
package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
)

const apiVersion = "1.0.0"

type model struct {
	Swagger  string `json:"swagger"`
	BasePath string `json:"basePath"`
	Info     struct {
		Title       string `json:"title"`
		Version     string `json:"version"`
		Description string `json:"description"`
	} `json:"info"`
	Schemes     []string               `json:"schemes"`
	Consumes    []string               `json:"consumes"`
	Produces    []string               `json:"produces"`
	Paths       map[string]interface{} `json:"paths"`
	Definitions map[string]interface{} `json:"definitions"`
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run main.go inputPath")
	}
	swagger := model{
		Swagger:     "2.0",
		Schemes:     []string{"http", "https"},
		Consumes:    []string{"application/json"},
		Produces:    []string{"application/json"},
		Paths:       make(map[string]interface{}),
		Definitions: make(map[string]interface{}),
	}
	swagger.Info.Title = "LoRa Server REST API"
	swagger.Info.Version = apiVersion
	swagger.Info.Description = `
For more information about the usage of the LoRa Server (REST) API, see
[https://docs.loraserver.io/loraserver/api/](https://docs.loraserver.io/loraserver/api/).
`

	fileInfos, err := ioutil.ReadDir(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	for _, fileInfo := range fileInfos {
		if !strings.HasSuffix(fileInfo.Name(), ".swagger.json") {
			continue
		}

		b, err := ioutil.ReadFile(path.Join(os.Args[1], fileInfo.Name()))
		if err != nil {
			log.Fatal(err)
		}

		// replace "title" by "description" for fields
		b = []byte(strings.Replace(string(b), `"title"`, `"description"`, -1))

		var m model
		err = json.Unmarshal(b, &m)
		if err != nil {
			log.Fatal(err)
		}

		for k, v := range m.Paths {
			swagger.Paths[k] = v
		}
		for k, v := range m.Definitions {
			swagger.Definitions[k] = v
		}
	}

	enc := json.NewEncoder(os.Stdout)
	err = enc.Encode(swagger)
	if err != nil {
		log.Fatal(err)
	}
}
