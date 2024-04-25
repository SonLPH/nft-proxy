package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	nft_proxy "github.com/alphabatem/nft-proxy"
	"github.com/babilu-online/common/context"
	"github.com/gagliardetto/solana-go"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type ImageService struct {
	context.DefaultService

	defaultSize int

	httpMedia *http.Client

	solSvc *SolanaImageService
	resize *ResizeService
	sql    *SqliteService
}

const IMG_SVC = "img_svc"

func (svc ImageService) Id() string {
	return IMG_SVC
}

func (svc *ImageService) Start() error {
	svc.solSvc = svc.Service(SOLANA_IMG_SVC).(*SolanaImageService)
	svc.sql = svc.Service(SQLITE_SVC).(*SqliteService)
	svc.resize = svc.Service(RESIZE_SVC).(*ResizeService)

	svc.httpMedia = &http.Client{Timeout: 10 * time.Second}

	svc.defaultSize = 720 //Gifs will be half the size

	return nil
}

func (svc *ImageService) Media(key string, skipCache bool) (*nft_proxy.Media, error) {
	if svc.IsSolKey(key) {
		return svc.solSvc.Media(key, skipCache)
	}

	return nil, errors.New("invalid key")
}

func (svc *ImageService) ImageFile(c *gin.Context, key string) error {
	var err error

	//Fetch the image file to see if its already in the system
	var media *nft_proxy.Media
	if svc.IsSolKey(key) {
		media, err = svc.solSvc.Media(key, false)
		if err != nil {
			return err
		}
	} else {
		return errors.New("unsupported chain")
	}

	cacheName := fmt.Sprintf("./cache/solana/%s.%s", media.Mint, media.ImageType)

	//Check for file or fetch
	ifo, err := os.Stat(cacheName)
	if err != nil || ifo.Size() == 0 { //Missing cached image
		err := svc.fetchMissingImage(media, cacheName)
		if err != nil {
			return err
		}
	}
	log.Printf("Using cached file: %s", cacheName)

	return svc.writeFile(c, cacheName, media)
}

func (svc *ImageService) ClearCache(key string) error {
	m, err := svc.solSvc.Media(key, false)
	if err != nil {
		return err
	}

	if time.Since(m.CreatedAt) < 3*time.Hour {
		return errors.New("cache recently cleared")
	}

	//Clear cached files
	_ = os.Remove(fmt.Sprintf("./cache/solana/%s.jpg", key))
	_ = os.Remove(fmt.Sprintf("./cache/solana/%s.png", key))

	//Clear
	return svc.solSvc.RemoveMedia(key)
}

func (svc *ImageService) writeFile(c *gin.Context, path string, media *nft_proxy.Media) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	ifo, err := file.Stat()
	modTime := time.Now()
	if ifo != nil {
		modTime = ifo.ModTime()
	}

	c.Header("Cache-Control", "public, max=age=172800")
	c.Header("Vary", "Accept-Encoding")
	c.Header("Last-Modified", modTime.Format("Mon, 02 Jan 2006 15:04:05 GMT")) //Mon, 03 Jun 2020 11:35:28 GMT
	c.Header("Content-Type", fmt.Sprintf("image/%s", media.ImageType))

	_, err = io.Copy(c.Writer, file)
	if err != nil {
		return err
	}
	return nil
}

func (svc *ImageService) fetchMissingImage(media *nft_proxy.Media, cacheName string) error {
	if media.ImageUri == "" {
		return errors.New("invalid image")
	}

	var err error
	var data []byte
	if strings.Contains(media.ImageUri, nft_proxy.BASE64_PREFIX) {
		base64String := media.ImageUri
		// Remove the data:image/jpeg;base64, prefix if present
		if v := strings.Index(base64String, nft_proxy.BASE64_PREFIX); v > -1 {
			base64String = base64String[v+len(nft_proxy.BASE64_PREFIX):]
		}

		data, err = base64.StdEncoding.DecodeString(base64String)
		if err != nil {
			return err
		}
	} else {
		//log.Println("Fetching", media.ImageUri)
		req, _ := http.NewRequest("GET", media.ImageUri, nil)
		req.Header.Set("User-Agent", "PostmanRuntime/7.29.2")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Encoding", "gzip,deflate,br")

		resp, err := svc.httpMedia.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return errors.New(resp.Status)
		}

		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
	}

	if len(data) == 0 {
		return errors.New("failed to download image")
	}

	output, err := os.Create(cacheName)
	if err != nil {
		return err
	}
	defer output.Close()

	log.Printf("Resizing file: %s", cacheName)
	err = svc.resize.Resize(data, output, svc.defaultSize)
	if err != nil {
		return err
	}
	return nil
}

func (svc *ImageService) MediaFile(c *gin.Context, key string) error {
	var media *nft_proxy.Media
	var err error
	if svc.IsSolKey(key) {
		media, err = svc.solSvc.Media(key, false)
		if err != nil {
			return err
		}
	}

	if media.MediaUri == "" {
		return errors.New("no media for mint")
	}

	resp, err := svc.httpMedia.Get(media.MediaUri)
	if err != nil {
		return err
	}

	//Write our data
	c.Header("Cache-Control", "public, max-age=31536000")
	c.Header("Expires", time.Now().AddDate(0, 1, 0).Format(http.TimeFormat))
	c.Header("Content-Type", media.MediaType)
	err = resp.Write(c.Writer)
	if err != nil {
		return err
	}

	return nil
}

func (svc *ImageService) IsSolKey(key string) bool {
	_, err := solana.PublicKeyFromBase58(key)
	return err == nil
}
