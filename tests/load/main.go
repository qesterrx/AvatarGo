package main

/*Нагрузочное тестирование*/

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/chai2010/webp"
)

func generateEmptyImage(width, height int, format string) ([]byte, string, string, error) {
	// Создаем пустое белое изображение
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Заполняем белым цветом
	white := color.RGBA{255, 255, 255, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, white)
		}
	}

	// Кодируем в нужный формат
	buf := new(bytes.Buffer)
	mimeType := ""
	extension := ""

	switch format {
	case "jpg", "jpeg":
		err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 90})
		if err != nil {
			return nil, mimeType, extension, err
		}
		mimeType = "image/jpeg"
		extension = "jpg"
	case "png":
		err := png.Encode(buf, img)
		if err != nil {
			return nil, mimeType, extension, err
		}
		mimeType = "image/png"
		extension = "png"
	case "webp":
		err := webp.Encode(buf, img, &webp.Options{Lossless: true})
		if err != nil {
			return nil, mimeType, extension, err
		}
		mimeType = "image/webp"
		extension = "webp"
	default:
		return nil, mimeType, extension, fmt.Errorf("unsupported format: %s", format)
	}

	return buf.Bytes(), mimeType, extension, nil
}

func generateUserID() string {
	// Генерируем случайный ID пользователя
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 10)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return "user-" + string(b)
}

func sendImageMultipart(port int, imageData []byte, filename, mimeType, userID string) error {
	url := fmt.Sprintf("http://localhost:%d/api/v1/avatars", port)

	// Создаем буфер для multipart данных
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Добавляем файл
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("failed to create form file: %v", err)
	}

	// Записываем изображение
	if _, err := io.Copy(part, bytes.NewReader(imageData)); err != nil {
		return fmt.Errorf("failed to write image: %v", err)
	}

	// Закрываем writer, чтобы завершить multipart данные
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %v", err)
	}

	// Создаем HTTP запрос
	req, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Отправляем запрос
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	fmt.Printf("[%s] Status: %s\n", userID, resp.Status)
	fmt.Printf("[%s] Response: %s\n", userID, string(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned error: %s", resp.Status)
	}

	return nil
}

func main() {

	var count int
	var port int
	var format string

	flag.IntVar(&count, "count", 1, "Количество циклов (сколько раз отправить изображение)")
	flag.IntVar(&port, "port", 8080, "Порт")
	flag.StringVar(&format, "format", "png", "Формат изображения: png, jpg, webp")

	flag.Parse()

	// Генерируем пустое изображение
	fmt.Printf("Generating empty %s image...\n", format)
	imageData, mimeType, extension, err := generateEmptyImage(500, 500, format)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	successCount := 0
	for i := 0; i < count; i++ {

		userID := generateUserID()
		filename := fmt.Sprintf("%s.%s", userID, extension)

		// Отправляем на сервер
		fmt.Printf("Sending to localhost:8080/api/v1/avatars with X-User-ID: %s\n", userID)
		if err := sendImageMultipart(port, imageData, filename, mimeType, userID); err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			successCount++
		}

		time.Sleep(10 * time.Millisecond)
	}

	fmt.Printf("Completed! Success: %d/%d\n", successCount, count)
}
