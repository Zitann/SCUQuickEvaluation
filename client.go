package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	captchaURL = "http://zhjw.scu.edu.cn/img/captcha.jpg"
	tokenURL   = "http://zhjw.scu.edu.cn/login"
	loginURL   = "http://zhjw.scu.edu.cn/j_spring_security_check"
	scoreURL   = "http://zhjw.scu.edu.cn/student/integratedQuery/scoreQuery/allTermScores/index"
	pjURL      = "http://zhjw.scu.edu.cn/student/teachingAssessment/evaluation/queryAll"
)

var (
	tokenRegex = regexp.MustCompile(`<input type="hidden" id="tokenValue" name="tokenValue" value="(.*?)">`)
)

// Course represents a course that needs evaluation
type Course struct {
	KCM   string `json:"KCM"`  // 课程名称
	KTID  string `json:"KTID"` // 课题ID
	WJBM  string `json:"WJBM"` // 问卷编码
	SFPG  string `json:"SFPG"` // 是否评估
	Index int    // 在列表中的索引
}

// EvaluationResponse represents the API response for evaluation list
type EvaluationResponse struct {
	Data struct {
		Records []Course `json:"records"`
	} `json:"data"`
}

// SCUClient handles all network operations for SCU educational system
type SCUClient struct {
	httpClient *http.Client
	Username   string
	Password   string
}

// NewSCUClient creates a new SCU client with cookie jar
func NewSCUClient() *SCUClient {
	jar, _ := cookiejar.New(nil)
	return &SCUClient{
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
	}
}

// SetCredentials sets the username and password
func (c *SCUClient) SetCredentials(username, password string) {
	c.Username = username
	c.Password = password
}

// GetCaptcha fetches the captcha image from the server
func (c *SCUClient) GetCaptcha() (image.Image, error) {
	req, err := http.NewRequest("GET", captchaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建验证码请求失败: %w", err)
	}
	c.setCommonHeaders(req, false)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取验证码失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取验证码失败，状态码: %d", resp.StatusCode)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取验证码内容失败: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("解码验证码图片失败: %w", err)
	}

	return img, nil
}

// GetToken fetches the login token from the server
func (c *SCUClient) GetToken() (string, error) {
	req, err := http.NewRequest("GET", tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建token请求失败: %w", err)
	}
	c.setCommonHeaders(req, false)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取token失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取token失败，状态码: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取token响应体失败: %w", err)
	}

	matches := tokenRegex.FindSubmatch(bodyBytes)
	if len(matches) < 2 {
		return "", fmt.Errorf("无法从页面提取token")
	}
	return string(matches[1]), nil
}

// Login performs the login operation
func (c *SCUClient) Login(captchaText string) (bool, error) {
	if c.Username == "" || c.Password == "" || captchaText == "" {
		return false, fmt.Errorf("学号、密码和验证码不能为空")
	}

	token, err := c.GetToken()
	if err != nil {
		return false, fmt.Errorf("获取登录token失败: %w", err)
	}

	// Hash password with MD5
	md5Hasher := md5.New()
	md5Hasher.Write([]byte(c.Password))
	hashedPassword := hex.EncodeToString(md5Hasher.Sum(nil))

	loginData := url.Values{}
	loginData.Set("tokenValue", token)
	loginData.Set("j_username", c.Username)
	loginData.Set("j_password", hashedPassword)
	loginData.Set("j_captcha", captchaText)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(loginData.Encode()))
	if err != nil {
		return false, fmt.Errorf("创建登录请求失败: %w", err)
	}
	c.setCommonHeaders(req, false)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取登录响应失败: %w", err)
	}

	bodyStr := string(bodyBytes)
	if strings.Contains(bodyStr, "欢迎您") || (resp.StatusCode == http.StatusOK && !strings.Contains(bodyStr, "错误提示") && !strings.Contains(bodyStr, "验证码不正确")) {
		return true, nil
	} else {
		if strings.Contains(bodyStr, "验证码不正确") {
			return false, fmt.Errorf("验证码不正确")
		} else if strings.Contains(bodyStr, "密码错误") {
			return false, fmt.Errorf("用户名或密码错误")
		}
		return false, fmt.Errorf("登录失败")
	}
}

// GetEvaluationList fetches the list of courses that need evaluation
func (c *SCUClient) GetEvaluationList() ([]Course, error) {
	data := url.Values{}
	data.Set("pageNum", "1")
	data.Set("pageSize", "30")
	data.Set("flag", "kt")

	req, err := http.NewRequest("POST", pjURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建评教列表请求失败: %w", err)
	}
	c.setCommonHeaders(req, false)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取评教列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取评教列表失败，状态码: %d", resp.StatusCode)
	}

	var evalResp EvaluationResponse
	if err := json.NewDecoder(resp.Body).Decode(&evalResp); err != nil {
		return nil, fmt.Errorf("解析评教列表响应失败: %w", err)
	}

	var pendingCourses []Course
	for i, course := range evalResp.Data.Records {
		if course.SFPG == "0" { // 未评教的课程
			course.Index = i
			pendingCourses = append(pendingCourses, course)
		}
	}

	return pendingCourses, nil
}

// EvaluateCourse performs evaluation for a specific course
func (c *SCUClient) EvaluateCourse(course Course) error {
	evaluationURL := fmt.Sprintf("http://zhjw.scu.edu.cn/student/teachingEvaluation/newEvaluation/evaluation/%s", course.KTID)

	// Get evaluation form
	req, err := http.NewRequest("GET", evaluationURL, nil)
	if err != nil {
		return fmt.Errorf("创建评教页面请求失败: %w", err)
	}
	c.setCommonHeaders(req, false)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("获取评教页面失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取评教页面失败，状态码: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取评教页面失败: %w", err)
	}

	// Parse HTML and extract form data
	formData, err := c.parseEvaluationForm(string(bodyBytes))
	if err != nil {
		return fmt.Errorf("解析评教表单失败: %w", err)
	}

	// Submit evaluation twice (as per original Python code)
	if err := c.submitEvaluation(formData, "0"); err != nil {
		return fmt.Errorf("第一次提交评教失败: %w", err)
	}

	time.Sleep(1 * time.Second)

	if err := c.submitEvaluation(formData, "1"); err != nil {
		return fmt.Errorf("第二次提交评教失败: %w", err)
	}

	return nil
}

// parseEvaluationForm parses the HTML form and extracts form data using regex
func (c *SCUClient) parseEvaluationForm(htmlContent string) (map[string]interface{}, error) {
	formData := make(map[string]interface{})

	// Extract token value
	tokenRegex := regexp.MustCompile(`<input type="hidden" name="tokenValue" value="(.*?)">`)
	if matches := tokenRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		formData["tokenValue"] = matches[1]
	}

	// Extract wjbm and ktid
	wjbmRegex := regexp.MustCompile(`<input[^>]*name="wjbm"[^>]*value="([^"]*)"`)
	if matches := wjbmRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		formData["wjbm"] = matches[1]
	}

	ktidRegex := regexp.MustCompile(`<input[^>]*name="ktid"[^>]*value="([^"]*)"`)
	if matches := ktidRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		formData["ktid"] = matches[1]
	}

	formData["tjcs"] = "0"

	// Parse radio buttons - select the first value for each name
	radioRegex := regexp.MustCompile(`<input[^>]*type="radio"[^>]*name="([^"]*)"[^>]*value="([^"]*)"`)
	radioMatches := radioRegex.FindAllStringSubmatch(htmlContent, -1)
	processedRadios := make(map[string]bool)
	for _, match := range radioMatches {
		if len(match) > 2 {
			name := match[1]
			value := match[2]
			if !processedRadios[name] {
				formData[name] = value
				processedRadios[name] = true
			}
		}
	}

	// Parse checkboxes - collect all values except "K_以上均无"
	checkboxRegex := regexp.MustCompile(`<input[^>]*type="checkbox"[^>]*name="([^"]*)"[^>]*value="([^"]*)"`)
	checkboxMatches := checkboxRegex.FindAllStringSubmatch(htmlContent, -1)
	checkboxData := make(map[string][]string)
	for _, match := range checkboxMatches {
		if len(match) > 2 {
			name := match[1]
			value := match[2]
			if value != "K_以上均无" {
				checkboxData[name] = append(checkboxData[name], value)
			}
		}
	}
	for name, values := range checkboxData {
		formData[name] = values
	}

	// Parse text inputs with placeholder
	textInputRegex := regexp.MustCompile(`<input[^>]*placeholder="请输入1-100的整数"[^>]*name="([^"]*)"`)
	textMatches := textInputRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, match := range textMatches {
		if len(match) > 1 {
			formData[match[1]] = "100"
		}
	}

	// Add text area comment
	textareaRegex := regexp.MustCompile(`<textarea name="([^"]*)" class="form-control value_element"[^>]*></textarea>`)
	if matches := textareaRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		formData[matches[1]] = "这门课程的教学效果很好,老师热爱教学,教学方式生动有趣,课程内容丰富且贴合时代特点。"
	}

	formData["compare"] = ""

	return formData, nil
}

// submitEvaluation submits the evaluation form
func (c *SCUClient) submitEvaluation(formData map[string]interface{}, tjcs string) error {
	formData["tjcs"] = tjcs

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add form fields
	for key, value := range formData {
		switch v := value.(type) {
		case string:
			if err := writer.WriteField(key, v); err != nil {
				return err
			}
		case []string:
			for _, item := range v {
				if err := writer.WriteField(key, item); err != nil {
					return err
				}
			}
		default:
			if err := writer.WriteField(key, fmt.Sprintf("%v", v)); err != nil {
				return err
			}
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}

	postURL := fmt.Sprintf("http://zhjw.scu.edu.cn/student/teachingAssessment/baseInformation/questionsAdd/doSave?tokenValue=%s", formData["tokenValue"])

	req, err := http.NewRequest("POST", postURL, &body)
	if err != nil {
		return fmt.Errorf("创建评教提交请求失败: %w", err)
	}

	c.setCommonHeaders(req, true)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("提交评教失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("提交评教失败，状态码: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析评教响应失败: %w", err)
	}

	if tjcs == "1" {
		if resultStr, ok := result["result"].(string); !ok || resultStr != "ok" {
			return fmt.Errorf("评教提交失败: %v", result)
		}
	} else {
		// Update token for second submission
		if token, ok := result["token"].(string); ok {
			formData["tokenValue"] = token
		}
	}

	return nil
}

// EvaluateAllCourses evaluates all pending courses
func (c *SCUClient) EvaluateAllCourses(courses []Course, progressCallback func(int, int, string)) error {
	total := len(courses)
	for i, course := range courses {
		if progressCallback != nil {
			progressCallback(i+1, total, fmt.Sprintf("正在评教: %s", course.KCM))
		}

		if err := c.EvaluateCourse(course); err != nil {
			if progressCallback != nil {
				progressCallback(i+1, total, fmt.Sprintf("评教失败: %s - %v", course.KCM, err))
			}
			continue
		}

		if progressCallback != nil {
			progressCallback(i+1, total, fmt.Sprintf("评教完成: %s", course.KCM))
		}

		// Add delay between evaluations
		time.Sleep(2 * time.Second)
	}
	return nil
}

// setCommonHeaders sets common HTTP headers for requests
func (c *SCUClient) setCommonHeaders(req *http.Request, isMultipart bool) {
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
	if isMultipart {
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	}
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("DNT", "1")
	req.Header.Set("Host", "zhjw.scu.edu.cn")
	if !isMultipart {
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0")
}

// Close closes the HTTP client
func (c *SCUClient) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
