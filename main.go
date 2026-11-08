package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

type Server struct {
	version         string
	port            int
	codeMap         map[string]Link
	symbolsAlphabet string
	codeLen         int
}

func NewServer(alphabet string, codeLen int) *Server {

	return &Server{
		symbolsAlphabet: alphabet,
		codeLen:         codeLen,
		codeMap:         make(map[string]Link),
	}
}

func (serv *Server) SaveLink(code string, link Link) {
	serv.codeMap[code] = link
}

type LinkPayload struct {
	URL string `json:"url"`
}

type Link struct {
	URL       string
	Code      string
	IP        string
	CreatedAt time.Time
}

type ShortLinkService struct {
	genRetryCount int
	alphabet      string
	codeLen       int
	codeMap       map[string]Link // Экземпляр мапы ссылок из памяти. Сервер из себя его передает сюда для работы.
}

func (sls *ShortLinkService) HandleURL(payload LinkPayload) (Link, error) {

	validUrl, err := ValidateURL(payload.URL)

	if err != nil {
		return Link{}, fmt.Errorf("validating url error: %w", err)
	}

	createdLink := Link{}

	for range sls.genRetryCount {
		newCode, err := GetRandomCode(sls.codeLen, sls.alphabet)

		if err != nil {
			return Link{}, fmt.Errorf("randoming code error: %w", err)
		}

		_, ok := sls.codeMap[newCode]

		if ok {
			continue
		}

		createdLink = Link{
			URL:       validUrl,
			Code:      newCode,
			IP:        "",
			CreatedAt: time.Now(),
		}

		sls.codeMap[newCode] = createdLink

		break
	}

	return createdLink, nil

}

func main() {

	serv := NewServer("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 7)
	serv.port = 8080
	serv.version = "0.01"

	testUrl := "https://www.google.com"

	validated, err := ValidateURL(testUrl)

	if err != nil {
		fmt.Printf("%s", "validating url error: "+err.Error())
		return
	}

	newCode, err := GetRandomCode(serv.codeLen, serv.symbolsAlphabet)

	if err != nil {
		fmt.Println("random code generating error: " + err.Error())
		return
	}

	serv.codeMap[newCode] = Link{
		URL:       validated,
		Code:      newCode,
		IP:        "",
		CreatedAt: time.Now(),
	}

	fmt.Println("Generated short code: ", newCode)
	fmt.Println("Formatted url: ", validated)

}

func ValidateURL(origin string) (string, error) {

	str := strings.TrimSpace(origin)
	strLen := len(str)
	if strLen < 1 {
		return "", errors.New("origin string is empty")
	}

	if strLen > 2048 {
		return "", errors.New("too long link")
	}

	strParsed, err := url.Parse(str)

	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	urlScheme := strings.ToLower((strParsed.Scheme))

	if urlScheme != "http" && urlScheme != "https" {
		return "", errors.New("unsupported protocol")
	}

	if strParsed.Host == "" {
		return "", errors.New("url host is empty")
	}

	strParsed.Scheme = urlScheme

	return strParsed.String(), nil
}

// GetRandomCode генерирует криптографически стойкую случайную строку заданной длины.
//
// Функция использует crypto/rand для получения энтропии, что делает результат
// непредсказуемым и пригодным для использования в качестве токенов сброса паролей,
// одноразовых кодов подтверждения (OTP) или идентификаторов сессий.
//
// ПРЕДУПРЕЖДЕНИЕ: Функция предполагает, что алфавит состоит исключительно из
// ASCII-символов (один символ = один байт). Если передать строку с многобайтовыми
// символами (например, кириллицу "абв" или эмодзи), индексация `alphabet[idx]`
// вернет случайный байт внутри руны, а не целый символ. Это приведет к появлению
// битых символов (Mojibake) в итоговой строке.
//
// Параметры:
//
//	codeLen — требуемая длина итоговой строки. Должна быть строго больше 0.
//	alphabet — строка допустимых символов для генерации. Должна содержать
//	           только ASCII-символы и иметь длину строго больше 0.
//
// Возвращаемые значения:
//
//	code — сгенерированная случайная строка.
//	err — не-nil, если аргументы невалидны (длины <= 0) или если источник
//	      энтропии (операционная система) вернул ошибку.
//
// Пример:
//
//	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
//	code, err := GetRandomCode(16, chars)
//	code: "4fG9kP2zX1bN8vR0"
func GetRandomCode(codeLen int, alphabet string) (string, error) {
	if codeLen < 1 || len(alphabet) < 1 {
		return "", errors.New("arguments lengths must be more than 1")
	}

	b := make([]byte, codeLen)
	alphabetLen := int64(len(alphabet))
	bInt := big.NewInt(alphabetLen)

	for i := range codeLen {
		// rand.Int выбирает число в полуинтервале [0, alphabetLen).
		// Использование big.Int здесь избыточно для алфавитов длиной < 2^63,
		// но гарантирует отсутствие статистического смещения (modulo bias).
		r, err := rand.Int(rand.Reader, bInt)
		if err != nil {
			return "", fmt.Errorf("error while random generating: %w", err)
		}
		b[i] = alphabet[r.Int64()]
	}

	return string(b), nil
}
