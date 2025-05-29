package main

import (
	"fmt"
	"image"
	"sync"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// AppState represents the current state of the application
type AppState int

const (
	StateLogin AppState = iota
	StateFetchingCaptcha
	StateCaptchaLoaded
	StateLoggingIn
	StateLoginFailed
	StateLoggedIn
	StateFetchingCourses
	StateCoursesLoaded
	StateEvaluating
	StateEvaluationComplete
)

// App represents the main application structure
type App struct {
	window *app.Window
	theme  *material.Theme

	// UI State
	currentState AppState
	mu           sync.Mutex

	// Login Widgets
	usernameEditor    widget.Editor
	passwordEditor    widget.Editor
	captchaEditor     widget.Editor
	loginButton       widget.Clickable
	refreshCaptchaBtn widget.Clickable

	// Course evaluation widgets
	courseList           widget.List
	selectAllButton      widget.Clickable
	evaluateButton       widget.Clickable
	backToLoginButton    widget.Clickable
	refreshCoursesButton widget.Clickable

	// Data
	client          *SCUClient
	statusMessage   string
	captchaImage    image.Image
	courses         []Course
	selectedCourses map[int]bool
	courseBools     []widget.Bool

	// Progress tracking
	evaluationProgress int
	evaluationTotal    int
	evaluationStatus   string
}

// NewApp creates a new application instance
func NewApp(w *app.Window) *App {
	th := material.NewTheme()

	a := &App{
		window:          w,
		theme:           th,
		client:          NewSCUClient(),
		currentState:    StateLogin,
		selectedCourses: make(map[int]bool),

		usernameEditor: widget.Editor{SingleLine: true, Submit: true},
		passwordEditor: widget.Editor{SingleLine: true, Submit: true, Mask: '*'},
		captchaEditor:  widget.Editor{SingleLine: true, Submit: true},

		courseList: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
	}

	// Initialize editors
	a.usernameEditor.SetText("")
	a.passwordEditor.SetText("")
	a.captchaEditor.SetText("")

	return a
}

// Run starts the application main loop
func (a *App) Run() error {
	var ops op.Ops

	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			a.client.Close()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.handleEvents(gtx)
			a.layoutUI(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// layoutUI handles the main UI layout based on current state
func (a *App) layoutUI(gtx layout.Context) {
	a.mu.Lock()
	state := a.currentState
	a.mu.Unlock()

	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Top:    unit.Dp(20),
			Bottom: unit.Dp(20),
			Left:   unit.Dp(20),
			Right:  unit.Dp(20),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			switch state {
			case StateLogin, StateFetchingCaptcha, StateCaptchaLoaded, StateLoggingIn, StateLoginFailed:
				return a.layoutLoginScreen(gtx)
			case StateLoggedIn, StateFetchingCourses, StateCoursesLoaded, StateEvaluating, StateEvaluationComplete:
				return a.layoutCourseScreen(gtx)
			default:
				return a.layoutLoginScreen(gtx)
			}
		})
	})
}

// layoutLoginScreen layouts the login screen
func (a *App) layoutLoginScreen(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(material.H3(a.theme, "SCU 快速评教系统").Layout),
		layout.Rigid(layout.Spacer{Height: unit.Dp(30)}.Layout),
		layout.Rigid(a.layoutLoginForm),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			a.mu.Lock()
			defer a.mu.Unlock()
			return material.Body1(a.theme, a.statusMessage).Layout(gtx)
		}),
	)
}

// layoutCourseScreen layouts the course evaluation screen
func (a *App) layoutCourseScreen(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	state := a.currentState
	courses := a.courses
	username := a.client.Username
	statusMsg := a.statusMessage
	progress := a.evaluationProgress
	total := a.evaluationTotal
	evalStatus := a.evaluationStatus
	a.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(material.H4(a.theme, fmt.Sprintf("欢迎您, %s", username)).Layout),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(material.Body1(a.theme, statusMsg).Layout),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		// Control buttons
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if state == StateCoursesLoaded {
								return material.Button(a.theme, &a.selectAllButton, "全选/取消全选").Layout(gtx)
							}
							return layout.Dimensions{}
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if state == StateCoursesLoaded {
								return material.Button(a.theme, &a.evaluateButton, "开始评教").Layout(gtx)
							}
							return layout.Dimensions{}
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if state == StateCoursesLoaded || state == StateEvaluationComplete {
								return material.Button(a.theme, &a.refreshCoursesButton, "刷新课程").Layout(gtx)
							}
							return layout.Dimensions{}
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, &a.backToLoginButton, "退出登录").Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		// Progress bar for evaluation
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state == StateEvaluating && total > 0 {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						progressText := fmt.Sprintf("评教进度: %d/%d", progress, total)
						return material.Body1(a.theme, progressText).Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(5)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Body2(a.theme, evalStatus).Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				)
			}
			return layout.Dimensions{}
		}),

		// Course list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(courses) == 0 {
				if state == StateFetchingCourses {
					return material.Body1(a.theme, "正在获取课程列表...").Layout(gtx)
				} else if state == StateLoggedIn {
					return material.Body1(a.theme, "无待评教课程").Layout(gtx)
				}
				return layout.Dimensions{}
			}

			return a.layoutCourseList(gtx, courses)
		}),
	)
}

// layoutLoginForm layouts the login form
func (a *App) layoutLoginForm(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Editor(a.theme, &a.usernameEditor, "学号").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Editor(a.theme, &a.passwordEditor, "密码").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(a.layoutCaptchaSection),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Button(a.theme, &a.loginButton, "登录").Layout(gtx)
		}),
	)
}

// layoutCaptchaSection layouts the captcha section
func (a *App) layoutCaptchaSection(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					a.mu.Lock()
					img := a.captchaImage
					state := a.currentState
					a.mu.Unlock()

					if img != nil {
						imgWidget := widget.Image{
							Src: paint.NewImageOp(img),
							Fit: widget.Contain,
						}
						return layout.UniformInset(unit.Dp(0)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = gtx.Dp(150)
							gtx.Constraints.Max.Y = gtx.Dp(50)
							return imgWidget.Layout(gtx)
						})
					} else if state == StateFetchingCaptcha {
						return material.Body2(a.theme, "正在加载验证码...").Layout(gtx)
					}
					return material.Body2(a.theme, "无法加载验证码").Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, &a.refreshCaptchaBtn, "刷新").Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Editor(a.theme, &a.captchaEditor, "验证码").Layout(gtx)
		}),
	)
}

// layoutCourseList layouts the course list
func (a *App) layoutCourseList(gtx layout.Context, courses []Course) layout.Dimensions {
	// Ensure we have enough bools for all courses
	a.mu.Lock()
	for len(a.courseBools) < len(courses) {
		a.courseBools = append(a.courseBools, widget.Bool{})
	}
	a.mu.Unlock()

	return material.List(a.theme, &a.courseList).Layout(gtx, len(courses), func(gtx layout.Context, i int) layout.Dimensions {
		if i >= len(courses) {
			return layout.Dimensions{}
		}

		course := courses[i]

		a.mu.Lock()
		isSelected := a.selectedCourses[i]
		a.courseBools[i].Value = isSelected
		a.mu.Unlock()

		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					checkbox := material.CheckBox(a.theme, &a.courseBools[i], "")
					return checkbox.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					text := fmt.Sprintf("%d. %s", i+1, course.KCM)
					return material.Body1(a.theme, text).Layout(gtx)
				}),
			)
		})
	})
}

// handleEvents handles UI events
func (a *App) handleEvents(gtx layout.Context) {
	a.mu.Lock()
	canInteract := a.currentState != StateLoggingIn && a.currentState != StateFetchingCaptcha && a.currentState != StateFetchingCourses && a.currentState != StateEvaluating
	state := a.currentState
	a.mu.Unlock()

	if canInteract {
		// Handle login screen events
		if state == StateLogin || state == StateCaptchaLoaded || state == StateLoginFailed {
			if a.refreshCaptchaBtn.Clicked(gtx) {
				go a.fetchCaptcha()
			}

			if a.loginButton.Clicked(gtx) {
				go a.performLogin()
			}

			// Handle editor submit events
			a.handleEditorEvents(gtx)
		}

		// Handle course screen events
		if state == StateLoggedIn || state == StateCoursesLoaded || state == StateEvaluationComplete {
			if a.backToLoginButton.Clicked(gtx) {
				a.backToLogin()
			}

			if a.refreshCoursesButton.Clicked(gtx) {
				go a.fetchCourses()
			}

			if state == StateCoursesLoaded {
				if a.selectAllButton.Clicked(gtx) {
					a.toggleSelectAll()
				}

				if a.evaluateButton.Clicked(gtx) {
					go a.startEvaluation()
				}

				// Handle course selection
				a.handleCourseSelection(gtx)
			}
		}
	}

	// Handle keyboard events
	a.handleKeyboardEvents(gtx, canInteract, state)
}

// handleEditorEvents handles editor submit events
func (a *App) handleEditorEvents(gtx layout.Context) {
	for {
		event, ok := a.usernameEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			go a.performLogin()
		}
	}

	for {
		event, ok := a.passwordEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			go a.performLogin()
		}
	}

	for {
		event, ok := a.captchaEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			go a.performLogin()
		}
	}
}

// handleCourseSelection handles course selection events
func (a *App) handleCourseSelection(gtx layout.Context) {
	a.mu.Lock()
	courses := a.courses
	a.mu.Unlock()

	for i := 0; i < len(courses) && i < len(a.courseBools); i++ {
		// 检查checkbox的值是否改变
		currentVal := a.courseBools[i].Value
		a.mu.Lock()
		prevVal := a.selectedCourses[i]
		if currentVal != prevVal {
			a.selectedCourses[i] = currentVal
		}
		a.mu.Unlock()
	}
}

// handleKeyboardEvents handles keyboard events
func (a *App) handleKeyboardEvents(gtx layout.Context, canInteract bool, state AppState) {
	// 简化键盘事件处理，由editor的submit事件处理回车键
}

// fetchCaptcha fetches captcha image
func (a *App) fetchCaptcha() {
	a.mu.Lock()
	if a.currentState == StateFetchingCaptcha {
		a.mu.Unlock()
		return
	}
	a.currentState = StateFetchingCaptcha
	a.statusMessage = "正在获取验证码..."
	a.captchaImage = nil
	a.mu.Unlock()
	a.window.Invalidate()

	img, err := a.client.GetCaptcha()
	if err != nil {
		a.setAppStatus(fmt.Sprintf("获取验证码失败: %v", err), StateLoginFailed)
		return
	}

	a.mu.Lock()
	a.captchaImage = img
	a.currentState = StateCaptchaLoaded
	a.statusMessage = "请输入验证码"
	a.mu.Unlock()
	a.window.Invalidate()
}

// performLogin performs login operation
func (a *App) performLogin() {
	a.mu.Lock()
	if a.currentState == StateLoggingIn {
		a.mu.Unlock()
		return
	}
	a.currentState = StateLoggingIn
	a.statusMessage = "正在登录..."
	username := a.usernameEditor.Text()
	password := a.passwordEditor.Text()
	captchaText := a.captchaEditor.Text()
	a.mu.Unlock()
	a.window.Invalidate()

	a.client.SetCredentials(username, password)

	success, err := a.client.Login(captchaText)
	if err != nil {
		a.setAppStatus(fmt.Sprintf("登录失败: %v", err), StateLoginFailed)
		go a.fetchCaptcha()
		return
	}

	if success {
		a.mu.Lock()
		a.passwordEditor.SetText("")
		a.captchaEditor.SetText("")
		a.currentState = StateLoggedIn
		a.statusMessage = fmt.Sprintf("登录成功! 欢迎您, %s", username)
		a.mu.Unlock()
		a.window.Invalidate()

		// Fetch courses after successful login
		go a.fetchCourses()
	} else {
		a.setAppStatus("登录失败", StateLoginFailed)
		go a.fetchCaptcha()
	}
}

// fetchCourses fetches course list
func (a *App) fetchCourses() {
	a.mu.Lock()
	if a.currentState == StateFetchingCourses {
		a.mu.Unlock()
		return
	}
	a.currentState = StateFetchingCourses
	a.statusMessage = "正在获取课程列表..."
	a.mu.Unlock()
	a.window.Invalidate()

	courses, err := a.client.GetEvaluationList()
	if err != nil {
		a.setAppStatus(fmt.Sprintf("获取课程列表失败: %v", err), StateLoggedIn)
		return
	}

	a.mu.Lock()
	a.courses = courses
	a.selectedCourses = make(map[int]bool)
	if len(courses) == 0 {
		a.currentState = StateLoggedIn
		a.statusMessage = "无待评教课程"
	} else {
		a.currentState = StateCoursesLoaded
		a.statusMessage = fmt.Sprintf("找到 %d 门待评教课程", len(courses))
	}
	a.mu.Unlock()
	a.window.Invalidate()
}

// toggleSelectAll toggles select all courses
func (a *App) toggleSelectAll() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if all are selected
	allSelected := true
	for i := 0; i < len(a.courses); i++ {
		if !a.selectedCourses[i] {
			allSelected = false
			break
		}
	}

	// Toggle all
	for i := 0; i < len(a.courses); i++ {
		a.selectedCourses[i] = !allSelected
	}
}

// startEvaluation starts the evaluation process
func (a *App) startEvaluation() {
	a.mu.Lock()
	var selectedCourses []Course
	for i, course := range a.courses {
		if a.selectedCourses[i] {
			selectedCourses = append(selectedCourses, course)
		}
	}
	a.mu.Unlock()

	if len(selectedCourses) == 0 {
		a.setAppStatus("请至少选择一门课程进行评教", StateCoursesLoaded)
		return
	}

	a.mu.Lock()
	a.currentState = StateEvaluating
	a.statusMessage = "正在进行评教..."
	a.evaluationProgress = 0
	a.evaluationTotal = len(selectedCourses)
	a.evaluationStatus = "准备开始评教..."
	a.mu.Unlock()
	a.window.Invalidate()

	err := a.client.EvaluateAllCourses(selectedCourses, func(current, total int, status string) {
		a.mu.Lock()
		a.evaluationProgress = current
		a.evaluationTotal = total
		a.evaluationStatus = status
		a.mu.Unlock()
		a.window.Invalidate()
	})

	if err != nil {
		a.setAppStatus(fmt.Sprintf("评教过程中发生错误: %v", err), StateEvaluationComplete)
	} else {
		a.setAppStatus("所有选中课程评教完成!", StateEvaluationComplete)
	}

	// Refresh courses after evaluation
	go a.fetchCourses()
}

// backToLogin returns to login screen
func (a *App) backToLogin() {
	a.mu.Lock()
	a.currentState = StateLogin
	a.statusMessage = ""
	a.courses = nil
	a.selectedCourses = make(map[int]bool)
	a.evaluationProgress = 0
	a.evaluationTotal = 0
	a.evaluationStatus = ""
	a.usernameEditor.SetText("")
	a.passwordEditor.SetText("")
	a.captchaEditor.SetText("")
	a.mu.Unlock()

	// Create new client to clear session
	a.client.Close()
	a.client = NewSCUClient()

	go a.fetchCaptcha()
	a.window.Invalidate()
}

// setAppStatus sets the application status
func (a *App) setAppStatus(message string, newState AppState) {
	a.mu.Lock()
	a.statusMessage = message
	a.currentState = newState
	a.mu.Unlock()
	a.window.Invalidate()
}
