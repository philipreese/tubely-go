package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type, only MP4 is allowed", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-go-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write video to disk", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to seek reset file pointer", err)
		return
	}


	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video aspect ratio", err)
		return
	}

	directory := ""
	switch aspectRatio {
	case "16:9":
		directory = "landscape/"
	case "9:16":
		directory = "portrait/"
	default:
		directory = "other/"
	}

	key := path.Join(directory, getAssetPath(mediaType))
	
	_, err = cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(cfg.s3Bucket),
		Key: aws.String(key),
		Body: tempFile,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to upload file to S3", err)
		return
	}

	videoURL := cfg.getObjectURL(key)
	video.VideoURL = &videoURL

	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	command := exec.Command("ffprobe","-v", "error", "-print_format", "json", "-show_streams", filePath)
	
	buffer := new(bytes.Buffer)
	command.Stdout = buffer
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("ffprobe error: %v", err)
	}

	type ffprobeOutStruct struct{
		Streams []struct{
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}

	decoder := json.NewDecoder(buffer)
	var ffprobeOut ffprobeOutStruct	
	if err := decoder.Decode(&ffprobeOut); err != nil {
		return "", fmt.Errorf("could not parse ffprobe output %v", err)
	}

	if len(ffprobeOut.Streams) == 0 {
		return "", errors.New("no video streams found")
	}
	
	ffprobeWidth := ffprobeOut.Streams[0].Width
	ffprobeHeight := ffprobeOut.Streams[0].Height

	aspectRatio := float64(ffprobeWidth) / float64(ffprobeHeight)
	tolerance := 0.1
	if math.Abs(aspectRatio - 1.78) <= tolerance {
		return "16:9", nil
	} else if math.Abs(aspectRatio - 0.56) <= tolerance {
		return "9:16", nil
	} else {
		return "other", nil
	}
}