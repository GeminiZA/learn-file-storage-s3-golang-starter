package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

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

	fmt.Println("uploading video", videoID, "by user", userID)

	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata", err)
		return
	}
	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized", err)
		return
	}

	const maxMemory = 10 << 20

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse form", err)
		return
	}

	file, fileHeaders, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get form file", err)
		return
	}
	defer file.Close()

	fileMediaType := fileHeaders.Header.Get("Content-Type")
	mt, _, err := mime.ParseMediaType(fileMediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couln't read Media Type", err)
		return
	}
	if mt != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}
	tempFile, err := os.CreateTemp(
		"", "tubely_upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couln't create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couln't write to temp file", err)
		return
	}

	proccessedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couln't process file for faststart", err)
		return
	}
	procFile, err := os.Open(proccessedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open procFile", err)
		return
	}
	defer procFile.Close()
	defer os.Remove(procFile.Name())

	s3FileNameExt := strings.Split(mt, "/")[1]
	randBytes := make([]byte, 32)
	_, err = rand.Read(randBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read rand", err)
		return
	}
	s3FileBaseName := hex.EncodeToString(randBytes)
	ratio, err := getVideoAspectRatio(procFile.Name())
	folder := "other"
	if ratio == "16:9" {
		folder = "landscape"
	} else if ratio == "9:16" {
		folder = "portrait"
	}
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Couln't get aspect ratio", err)
		return
	}
	s3FileName := fmt.Sprintf("%s/%s.%s", folder, s3FileBaseName, s3FileNameExt)
	poinput := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &s3FileName,
		Body:        procFile,
		ContentType: &mt,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &poinput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload file", err)
		return
	}

	newVideoUrl := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, s3FileName)
	metadata.VideoURL = &newVideoUrl
	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update database", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}
