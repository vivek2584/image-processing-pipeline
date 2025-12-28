package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"strings"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/gographics/imagick.v2/imagick"
)

type ConvertImageCommand func(args []string) (*imagick.ImageCommandResult, error)

type Converter struct {
	cmd ConvertImageCommand
}

func (c *Converter) Grayscale(inputFilepath string, outputFilepath string) error {
	// Convert the image to grayscale using imagemagick
	// We are directly calling the convert command
	_, err := c.cmd([]string{
		"convert", inputFilepath, "-set", "colorspace", "Gray", outputFilepath,
	})
	return err
}

func main() {
	// Accept --input and --output arguments for the images
	inputCSVPath := flag.String("input", "", "path to csv containing links to be processed")
	outputDirPath := flag.String("output", "", "A path to where the processed image should be written")
	flag.Parse()

	// Ensure that both flags were set
	if *inputCSVPath == "" || *outputDirPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	// read csv to get the image links
	imageLinks, err := readCSV(*inputCSVPath)
	if err != nil {
		log.Println(err)
		return
	}

	// Set up imagemagick
	imagick.Initialize()
	defer imagick.Terminate()

	// Build a Converter struct that will use imagick
	converter := &Converter{
		cmd: imagick.ConvertImageCommand,
	}

	// concurrently download all images and process them as they finish downloading
	var downloadWG sync.WaitGroup
	var processWG sync.WaitGroup

	downloadedImage := make(chan string)

	for id, link := range imageLinks {
		downloadWG.Add(1)
		go func(id int, link string, downloadedImage chan<- string) {
			defer downloadWG.Done()

			filename := fmt.Sprintf("image%d.jpg", id)

			err := GetImageFromLink("inputs", filename, link)
			if err != nil {
				log.Println(err)
				return
			}

			downloadedImage <- filepath.Join("inputs", filename)
		}(id, link, downloadedImage)
	}

	go func() {
		downloadWG.Wait()
		close(downloadedImage)
	}()

	processWG.Add(runtime.NumCPU())
	for process := 1; process <= runtime.NumCPU(); process++ {
		go func() {
			defer processWG.Done()

			for image := range downloadedImage {
				err := ProcessImage(converter, image, *outputDirPath)
				if err != nil {
					log.Println(err)
				}
			}
		}()
	}

	processWG.Wait()
}

func readCSV(csvFilePath string) ([]string, error) {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	var imageLinks []string

	reader := csv.NewReader(file)

	for {
		imageLink, err := reader.Read()

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		imageLinks = append(imageLinks, imageLink...)
	}

	return imageLinks, nil
}

func GetImageFromLink(dir, filename, link string) error {
	response, err := http.Get(link)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	imagePath := filepath.Join(dir, filename)
	file, err := os.Create(imagePath)

	if err != nil {
		return err
	}

	defer file.Close()

	_, err = io.Copy(file, response.Body)
	return err
}

func ProcessImage(converter *Converter, inputImagePath string, outputDirPath string) error {

	base := filepath.Base(inputImagePath) 
	ext := filepath.Ext(base)             
	name := strings.TrimSuffix(base, ext) 

	outputImagePath := filepath.Join(
		outputDirPath,
		name+"-grayscaled"+ext,
	)

	log.Printf("processing: %q to %q\n", inputImagePath, outputImagePath)
	// Do the conversion
	return converter.Grayscale(inputImagePath, outputImagePath)
}
