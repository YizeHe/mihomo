package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/log"
)

const mihomoAPI = "http://127.0.0.1:19090"

type tangentApp struct {
	fyneApp   fyne.App
	mainWin   fyne.Window
	token     string
	email     string
	expiresAt string
	proxyOn   bool
	prevMode  string
}

// ---------- File logger ----------

var logFile *os.File

func initLogger() {
	dir := sessionDir()
	os.MkdirAll(dir, 0700)
	f, err := os.OpenFile(filepath.Join(dir, "run.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open log file: %v\n", err)
		return
	}
	logFile = f
}

func logf(format string, args ...interface{}) {
	msg := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
	fmt.Print(msg)
	if logFile != nil {
		logFile.WriteString(msg)
		logFile.Sync()
	}
}

// ---------- Session persistence ----------

type sessionData struct {
	Email     string `json:"email"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func sessionDir() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData, _ = os.Getwd()
	}
	return filepath.Join(appData, "tangentproxy")
}

func sessionPath() string {
	return filepath.Join(sessionDir(), "session.json")
}

func saveSession(s *sessionData) error {
	dir := sessionDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), data, 0600)
}

func loadSession() *sessionData {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		return nil
	}
	var s sessionData
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	if s.Token == "" || s.Email == "" {
		return nil
	}
	return &s
}

func deleteSession() error {
	return os.Remove(sessionPath())
}

// ---------- mihomo API helpers ----------

func mihomoGet(path string) (map[string]interface{}, error) {
	resp, err := http.Get(mihomoAPI + path)
	if err != nil {
		return nil, fmt.Errorf("mihomo API error: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response error: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response error: %w", err)
	}
	return result, nil
}

func mihomoPatch(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PATCH", mihomoAPI+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func mihomoPut(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", mihomoAPI+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ---------- Backend API helpers ----------

type loginResponse struct {
	Token     string `json:"token"`
	Email     string `json:"email"`
	ExpiresAt string `json:"expires_at"`
	Error     string `json:"error"`
}

func backendLogin(email, password string) (*loginResponse, error) {
	payload := map[string]string{"email": email, "password": password}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(BackendURL+"/api/login", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("server connection failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result loginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response error: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf(result.Error)
	}
	if result.Token == "" {
		return nil, fmt.Errorf("login failed: no token received")
	}
	return &result, nil
}

func backendActivate(code string) (*loginResponse, error) {
	payload := map[string]string{"code": code}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(BackendURL+"/api/activate", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("server connection failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result loginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response error: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf(result.Error)
	}
	if result.Token == "" {
		return nil, fmt.Errorf("activation failed: no token received")
	}
	return &result, nil
}

func backendRegister(email, password, inviteCode string) error {
	payload := map[string]string{"email": email, "password": password, "invite_code": inviteCode}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(BackendURL+"/api/register", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("server connection failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse response error: %w", err)
	}
	if e, ok := result["error"]; ok && e != nil && e.(string) != "" {
		return fmt.Errorf(e.(string))
	}
	return nil
}

// ---------- runGUI ----------

func runGUI() {
	initLogger()
	logf("TangentVPN starting, BackendURL=%s", BackendURL)

	// Find Chinese font
	for _, f := range []string{
		`C:\Windows\Fonts\msyh.ttc`,
		`C:\Windows\Fonts\simhei.ttf`,
		`C:\Windows\Fonts\simsun.ttc`,
	} {
		if _, err := os.Stat(f); err == nil {
			os.Setenv("FYNE_FONT", f)
			logf("Using font: %s", f)
			break
		}
	}

	fyneApp := app.New()
	mainWin := fyneApp.NewWindow("TangentVPN")
	mainWin.Resize(fyne.NewSize(500, 600))
	mainWin.CenterOnScreen()

	ta := &tangentApp{
		fyneApp: fyneApp,
		mainWin: mainWin,
		prevMode: "rule",
	}

	// Check for saved session
	saved := loadSession()
	if saved != nil {
		logf("Found saved session for: %s", saved.Email)
		ta.email = saved.Email
		ta.token = saved.Token
		ta.expiresAt = saved.ExpiresAt
		ta.mainWin.SetTitle("TangentVPN - " + saved.Email)
		ta.mainWin.SetContent(ta.buildLoadingContent())
		go ta.startMihomo()
	} else {
		ta.showActivationScreen()
	}

	mainWin.ShowAndRun()
}

// ---------- Activation Screen (Primary) ----------

func (ta *tangentApp) showActivationScreen() {
	ta.mainWin.SetTitle("TangentVPN")

	title := widget.NewLabel("TangentVPN")
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	codeEntry := widget.NewEntry()
	codeEntry.SetPlaceHolder("Enter your activation code")

	activateBtn := widget.NewButton("Activate", nil)
	activateBtn.Importance = widget.HighImportance

	switchBtn := widget.NewButton("Switch to Login", nil)
	switchBtn.Importance = widget.LowImportance

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	activateBtn.OnTapped = func() {
		code := strings.TrimSpace(codeEntry.Text)
		if code == "" {
			statusLabel.SetText("Please enter your activation code")
			return
		}

		activateBtn.Disable()
		statusLabel.SetText("Activating...")

		go func() {
			logf("Activation attempt: %s", code)
			result, err := backendActivate(code)
			if err != nil {
				logf("Activation failed: %v", err)
				statusLabel.SetText("Activation failed: " + err.Error())
				activateBtn.Enable()
				canvas.Refresh(statusLabel)
				canvas.Refresh(activateBtn)
				return
			}

			ta.token = result.Token
			ta.email = result.Email
			ta.expiresAt = result.ExpiresAt

			// Save session
			if err := saveSession(&sessionData{
				Email:     result.Email,
				Token:     result.Token,
				ExpiresAt: result.ExpiresAt,
			}); err != nil {
				logf("WARN: save session failed: %v", err)
			}

			logf("Activation OK: %s, expires: %s", result.Email, result.ExpiresAt)

			ta.mainWin.SetContent(ta.buildLoadingContent())
			ta.mainWin.SetTitle("TangentVPN - " + result.Email)
			go ta.startMihomo()
		}()
	}

	switchBtn.OnTapped = func() {
		ta.showLoginWindow()
	}

	form := container.NewVBox(
		layout.NewSpacer(),
		title,
		widget.NewLabel(""),
		widget.NewLabel("Activation Code"),
		codeEntry,
		widget.NewLabel(""),
		activateBtn,
		statusLabel,
		layout.NewSpacer(),
		container.NewHBox(layout.NewSpacer(), switchBtn),
	)

	padded := container.NewPadded(form)
	ta.mainWin.SetContent(padded)
}

// ---------- Login Window (Secondary - via Switch button) ----------

func (ta *tangentApp) showLoginWindow() {
	ta.mainWin.SetTitle("TangentVPN - Login")

	title := widget.NewLabel("TangentVPN")
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder("Email")

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Password")

	loginBtn := widget.NewButton("Login", nil)
	loginBtn.Importance = widget.HighImportance

	registerBtn := widget.NewButton("Register", nil)
	registerBtn.Importance = widget.LowImportance

	switchBtn := widget.NewButton("Back to Activation", nil)
	switchBtn.Importance = widget.LowImportance

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	loginBtn.OnTapped = func() {
		email := strings.TrimSpace(emailEntry.Text)
		password := passwordEntry.Text
		if email == "" || password == "" {
			statusLabel.SetText("Please enter email and password")
			return
		}

		loginBtn.Disable()
		statusLabel.SetText("Logging in...")

		go func() {
			logf("Login attempt: %s", email)
			result, err := backendLogin(email, password)
			if err != nil {
				logf("Login failed: %v", err)
				statusLabel.SetText("Login failed: " + err.Error())
				loginBtn.Enable()
				canvas.Refresh(statusLabel)
				canvas.Refresh(loginBtn)
				return
			}

			ta.token = result.Token
			ta.email = result.Email
			ta.expiresAt = result.ExpiresAt

			if err := saveSession(&sessionData{
				Email:     result.Email,
				Token:     result.Token,
				ExpiresAt: result.ExpiresAt,
			}); err != nil {
				logf("WARN: save session failed: %v", err)
			}

			logf("Login OK: %s", result.Email)

			ta.mainWin.SetContent(ta.buildLoadingContent())
			ta.mainWin.SetTitle("TangentVPN - " + result.Email)
			go ta.startMihomo()
		}()
	}

	registerBtn.OnTapped = func() {
		ta.showRegistrationDialog()
	}

	switchBtn.OnTapped = func() {
		ta.showActivationScreen()
	}

	form := container.NewVBox(
		layout.NewSpacer(),
		title,
		widget.NewLabel(""),
		widget.NewLabel("Email"),
		emailEntry,
		widget.NewLabel("Password"),
		passwordEntry,
		widget.NewLabel(""),
		loginBtn,
		registerBtn,
		statusLabel,
		layout.NewSpacer(),
		container.NewHBox(layout.NewSpacer(), switchBtn),
	)

	padded := container.NewPadded(form)
	ta.mainWin.SetContent(padded)
}

// ---------- Registration Dialog ----------

func (ta *tangentApp) showRegistrationDialog() {
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder("Email")

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Password")

	confirmEntry := widget.NewPasswordEntry()
	confirmEntry.SetPlaceHolder("Confirm Password")

	inviteEntry := widget.NewEntry()
	inviteEntry.SetPlaceHolder("Optional - $5 discount")

	items := []*widget.FormItem{
		{Text: "Email", Widget: emailEntry},
		{Text: "Password", Widget: passwordEntry},
		{Text: "Confirm", Widget: confirmEntry},
		{Text: "Invite Code", Widget: inviteEntry},
	}

	dialog.ShowForm("Register", "Register", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		email := strings.TrimSpace(emailEntry.Text)
		password := passwordEntry.Text
		confirm := confirmEntry.Text
		inviteCode := strings.TrimSpace(inviteEntry.Text)

		if email == "" || password == "" {
			dialog.ShowError(fmt.Errorf("please fill all fields"), ta.mainWin)
			return
		}
		if password != confirm {
			dialog.ShowError(fmt.Errorf("passwords do not match"), ta.mainWin)
			return
		}
		if len(password) < 6 {
			dialog.ShowError(fmt.Errorf("password must be at least 6 characters"), ta.mainWin)
			return
		}

		go func() {
			err := backendRegister(email, password, inviteCode)
			if err != nil {
				dialog.ShowError(fmt.Errorf("registration failed: %s", err.Error()), ta.mainWin)
				return
			}
			dialog.ShowInformation("Registration Successful",
				"Your account has no active plan yet.\n\n"+
					"Please contact the admin to purchase:\n"+
					"  Email: tangent2533@gmail.com\n"+
					"  WeChat: tangentlab", ta.mainWin)
		}()
	}, ta.mainWin)
}

// ---------- Loading Screen ----------

func (ta *tangentApp) buildLoadingContent() fyne.CanvasObject {
	label := widget.NewLabel("Starting proxy engine...")
	label.Alignment = fyne.TextAlignCenter
	return container.NewCenter(label)
}

// ---------- Start mihomo ----------

func (ta *tangentApp) startMihomo() {
	options := []hub.Option{
		hub.WithExternalController("127.0.0.1:19090"),
		hub.WithSecret(""), // disable API auth for local GUI
	}

	logf("Starting mihomo, config size: %d bytes", len(embeddedConfig))

	if err := hub.Parse(embeddedConfig, options...); err != nil {
		logf("ERROR hub.Parse failed: %v", err)
		ta.showErrorAndRetry(fmt.Errorf("proxy engine failed: %s", err.Error()))
		return
	}

	logf("hub.Parse OK, waiting for API at %s...", mihomoAPI)

	ready := false
	for i := 0; i < 30; i++ {
		resp, err := http.Get(mihomoAPI + "/version")
		if err == nil {
			resp.Body.Close()
			logf("API check %d: status %d", i, resp.StatusCode)
			if resp.StatusCode == 200 {
				ready = true
				break
			}
		} else {
			logf("API check %d: %v", i, err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !ready {
		logf("ERROR: mihomo API not ready after 15s")
		ta.showErrorAndRetry(fmt.Errorf("proxy engine timeout, please restart"))
		return
	}

	logf("mihomo API ready")

	if NoSysProxy {
		logf("Skipping system proxy (-n flag)")
	} else {
		if err := SetSystemProxy(); err != nil {
			logf("WARN SetSystemProxy failed: %v", err)
		} else {
			logf("System proxy enabled")
		}
	}
	ta.proxyOn = true

	logf("Showing main panel")
	ta.mainWin.SetContent(ta.buildMainContent())
}

func (ta *tangentApp) showErrorAndRetry(err error) {
	logf("Proxy error: %s", err.Error())

	title := widget.NewLabel("Error")
	title.TextStyle = fyne.TextStyle{Bold: true}

	errLabel := widget.NewLabel(err.Error())
	errLabel.Wrapping = fyne.TextWrapWord

	retryBtn := widget.NewButton("Retry", nil)
	retryBtn.Importance = widget.HighImportance
	retryBtn.OnTapped = func() {
		ta.mainWin.SetContent(ta.buildLoadingContent())
		go ta.startMihomo()
	}

	logoutBtn := widget.NewButton("Logout", nil)
	logoutBtn.Importance = widget.DangerImportance
	logoutBtn.OnTapped = func() {
		deleteSession()
		ta.token = ""
		ta.email = ""
		ta.expiresAt = ""
		ta.proxyOn = false
		ta.showActivationScreen()
	}

	content := container.NewVBox(
		layout.NewSpacer(),
		title,
		errLabel,
		widget.NewLabel(""),
		retryBtn,
		logoutBtn,
		layout.NewSpacer(),
	)

	ta.mainWin.SetContent(container.NewPadded(content))
}

// ---------- Main Window ----------

func (ta *tangentApp) buildMainContent() fyne.CanvasObject {
	// Top bar
	emailLabel := widget.NewLabel(ta.email)
	emailLabel.TextStyle = fyne.TextStyle{Bold: true}

	exitBtn := widget.NewButton("Exit", nil)
	exitBtn.Importance = widget.DangerImportance
	exitBtn.OnTapped = func() {
		UnsetSystemProxy()
		executor.Shutdown()
		ta.fyneApp.Quit()
	}

	logoutBtn := widget.NewButton("Logout", nil)
	logoutBtn.OnTapped = func() {
		UnsetSystemProxy()
		deleteSession()
		ta.token = ""
		ta.email = ""
		ta.expiresAt = ""
		ta.proxyOn = false
		ta.showActivationScreen()
	}

	topBar := container.NewHBox(
		emailLabel,
		layout.NewSpacer(),
		logoutBtn,
		exitBtn,
	)

	// Expiry warning
	var expiryWarning fyne.CanvasObject
	if ta.isExpired() {
		warnLabel := widget.NewLabel("Plan expired. Please activate a new code or contact tangent2533@gmail.com")
		warnLabel.Wrapping = fyne.TextWrapWord
		warnLabel.Importance = widget.HighImportance

		buyBtn := widget.NewButton("Buy New Code", nil)
		buyBtn.Importance = widget.HighImportance
		buyBtn.OnTapped = func() {
			openBrowser("https://tangentlab2533.github.io")
		}

		newCodeBtn := widget.NewButton("Enter New Code", nil)
		newCodeBtn.OnTapped = func() {
			UnsetSystemProxy()
			deleteSession()
			ta.token = ""
			ta.email = ""
			ta.expiresAt = ""
			ta.proxyOn = false
			ta.showActivationScreen()
		}

		expiryWarning = container.NewVBox(warnLabel, buyBtn, newCodeBtn)
	}

	// Proxy status
	proxyStatusLabel := widget.NewLabel("Proxy Status: ON")
	proxyStatusLabel.TextStyle = fyne.TextStyle{Bold: true}
	proxyStatusLabel.Alignment = fyne.TextAlignCenter

	proxyToggleBtn := widget.NewButton("Disable Proxy", nil)
	proxyToggleBtn.Importance = widget.DangerImportance

	// Mode
	modeLabel := widget.NewLabel("Proxy Mode")
	modeSelect := widget.NewSelect([]string{"Rule", "Global", "Direct"}, nil)
	modeSelect.SetSelected("Rule")

	// Node
	groupLabel := widget.NewLabel("Node Selection")
	groupSelect := widget.NewSelect([]string{}, nil)
	memberSelect := widget.NewSelect([]string{}, nil)

	// Info
	infoLabel := widget.NewLabel("Proxy: HTTP=127.0.0.1:7899 SOCKS5=127.0.0.1:7898")
	if NoSysProxy {
		infoLabel = widget.NewLabel("Proxy: HTTP=127.0.0.1:7899 SOCKS5=127.0.0.1:7898 (system proxy disabled)")
	}

	// --- Callbacks ---

	proxyToggleBtn.OnTapped = func() {
		if ta.proxyOn {
			currentMode := modeToAPI(modeSelect.Selected)
			ta.prevMode = currentMode

			if err := mihomoPatch("/configs", map[string]string{"mode": "direct"}); err != nil {
				dialog.ShowError(fmt.Errorf("switch mode failed: %s", err.Error()), ta.mainWin)
				return
			}
			if !NoSysProxy {
				UnsetSystemProxy()
			}
			ta.proxyOn = false
			proxyStatusLabel.SetText("Proxy Status: OFF")
			proxyToggleBtn.SetText("Enable Proxy")
			proxyToggleBtn.Importance = widget.HighImportance
		} else {
			mode := ta.prevMode
			if mode == "" {
				mode = "rule"
			}
			if err := mihomoPatch("/configs", map[string]string{"mode": mode}); err != nil {
				dialog.ShowError(fmt.Errorf("switch mode failed: %s", err.Error()), ta.mainWin)
				return
			}
			if !NoSysProxy {
				SetSystemProxy()
			}
			ta.proxyOn = true
			proxyStatusLabel.SetText("Proxy Status: ON")
			proxyToggleBtn.SetText("Disable Proxy")
			proxyToggleBtn.Importance = widget.DangerImportance
			modeSelect.SetSelected(apiToMode(mode))
		}
		canvas.Refresh(proxyToggleBtn)
		canvas.Refresh(proxyStatusLabel)
	}

	modeSelect.OnChanged = func(selected string) {
		apiMode := modeToAPI(selected)
		if err := mihomoPatch("/configs", map[string]string{"mode": apiMode}); err != nil {
			dialog.ShowError(fmt.Errorf("switch mode failed: %s", err.Error()), ta.mainWin)
			return
		}
		if ta.proxyOn {
			ta.prevMode = apiMode
		}
		log.Infoln("Mode changed to: %s", apiMode)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		groups := ta.loadProxyGroups()
		groupSelect.Options = groups
		canvas.Refresh(groupSelect)
	}()

	groupSelect.OnChanged = func(selected string) {
		if selected == "" {
			return
		}
		members := ta.loadGroupMembers(selected)
		memberSelect.Options = members
		memberSelect.ClearSelected()
		canvas.Refresh(memberSelect)
	}

	memberSelect.OnChanged = func(selected string) {
		if selected == "" || groupSelect.Selected == "" {
			return
		}
		groupName := groupSelect.Selected
		go func() {
			err := mihomoPut("/proxies/"+groupName, map[string]string{"name": selected})
			if err != nil {
				dialog.ShowError(fmt.Errorf("switch node failed: %s", err.Error()), ta.mainWin)
				return
			}
			log.Infoln("Switched node: %s -> %s", groupName, selected)
		}()
	}

	// Build cards
	proxyCard := widget.NewCard("Proxy Status", "",
		container.NewVBox(
			proxyStatusLabel,
			layout.NewSpacer(),
			proxyToggleBtn,
		),
	)

	modeCard := widget.NewCard("Proxy Mode", "",
		container.NewVBox(
			modeLabel,
			modeSelect,
		),
	)

	nodeCard := widget.NewCard("Node Selection", "",
		container.NewVBox(
			groupLabel,
			groupSelect,
			memberSelect,
		),
	)

	content := container.NewVBox(
		topBar,
		widget.NewSeparator(),
	)

	if expiryWarning != nil {
		content.Add(expiryWarning)
	}

	content.Add(proxyCard)
	content.Add(modeCard)
	content.Add(nodeCard)
	content.Add(widget.NewSeparator())
	content.Add(infoLabel)

	scrollContent := container.NewVScroll(content)
	scrollContent.SetMinSize(fyne.NewSize(500, 600))

	return scrollContent
}

// ---------- Proxy group helpers ----------

func (ta *tangentApp) loadProxyGroups() []string {
	data, err := mihomoGet("/proxies")
	if err != nil {
		log.Warnln("Failed to load proxies: %s", err.Error())
		return nil
	}

	proxies, ok := data["proxies"].(map[string]interface{})
	if !ok {
		return nil
	}

	var groups []string
	for name, v := range proxies {
		p, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		pType, _ := p["type"].(string)
		switch pType {
		case "Selector", "URLTest", "Fallback", "LoadBalance":
			groups = append(groups, name)
		}
	}
	return groups
}

func (ta *tangentApp) loadGroupMembers(groupName string) []string {
	data, err := mihomoGet("/proxies/" + groupName)
	if err != nil {
		log.Warnln("Failed to load group members: %s", err.Error())
		return nil
	}

	all, ok := data["all"].([]interface{})
	if !ok {
		return nil
	}

	var members []string
	for _, v := range all {
		if name, ok := v.(string); ok {
			members = append(members, name)
		}
	}
	return members
}

// ---------- Mode mapping ----------

func modeToAPI(display string) string {
	switch display {
	case "Rule":
		return "rule"
	case "Global":
		return "global"
	case "Direct":
		return "direct"
	default:
		return "rule"
	}
}

func apiToMode(api string) string {
	switch api {
	case "rule":
		return "Rule"
	case "global":
		return "Global"
	case "direct":
		return "Direct"
	default:
		return "Rule"
	}
}

// ---------- Expiry check ----------

func (ta *tangentApp) isExpired() bool {
	if ta.expiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, ta.expiresAt)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z07:00", ta.expiresAt)
		if err != nil {
			t, err = time.Parse("2006-01-02", ta.expiresAt)
			if err != nil {
				return false
			}
		}
	}
	return time.Now().After(t)
}

// ---------- Browser helper ----------

func openBrowser(url string) {
	var cmd string
	var args []string
	switch {
	case strings.Contains(strings.ToLower(os.Getenv("OS")), "windows"):
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case fileExists("/usr/bin/open"):
		cmd = "/usr/bin/open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	exec.Command(cmd, args...).Start()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
