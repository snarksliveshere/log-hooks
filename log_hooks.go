package log_hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var errStore = &mailErrStore{errToTime: make(map[string]time.Time), generalErr: "general"}

type mailErrStore struct {
	errToTime   map[string]time.Time
	generalErr  string
	errToTimeMu sync.RWMutex
}

// MailHook to sends logs by email without authentication.
type MailHook struct {
	appName   string
	host      string
	port      int
	sender    string
	recipient string
}

// MailAuthHook to sends logs by email with authentication.
type MailAuthHook struct {
	appName   string
	host      string
	port      int
	sender    string
	recipient string
	username  string
	password  string
}

type StderrHook struct {
	textFormater *logrus.TextFormatter
}

// 1) set output format to stdout [text|json]
// 2) set verbosity [panic|fatal|error|warn|info|debug|trace]
// 3) sending errors to emails [panic|fatal|error|warn]
// 4) sending logs to stdout [info|debug|trace|panic|fatal|error|warn] and errors to stderr [panic|fatal|error|warn]
func UsefulSetupLogrus(
	log *logrus.Logger,
	mailHostPort string,
	format string,
	level string,
	appName string,
	sender string,
	recipient string,
) error {
	log.Out = os.Stdout

	host, strPort, err := net.SplitHostPort(mailHostPort)
	if err != nil {
		return err
	}

	port, err := strconv.Atoi(strPort)
	if err != nil {
		return err
	}

	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	log.SetLevel(logLevel)

	stderrHook, err := NewStderrHook()
	if err != nil {
		return err
	}
	log.Hooks.Add(stderrHook)

	mailHook, err := NewMailHook(appName, host, port, sender, recipient)
	if err != nil {
		return err
	}
	log.Hooks.Add(mailHook)

	if format == "json" {
		log.SetFormatter(&logrus.JSONFormatter{})
	} else {
		customFormatter := &logrus.TextFormatter{}
		customFormatter.FullTimestamp = true
		log.SetFormatter(customFormatter)
	}
	return nil
}

// NewMailHook creates a hook to be added to an instance of logger.
func NewMailHook(appname string, host string, port int, sender string, recipient string) (*MailHook, error) {
	err := checkMailHookParams(host, port, sender, recipient)
	if err != nil {
		return nil, err
	}

	return &MailHook{
		appName:   appname,
		host:      host,
		port:      port,
		sender:    sender,
		recipient: recipient,
	}, nil
}

// NewMailAuthHook creates a hook to be added to an instance of logger.
//func NewMailAuthHook(appName string, host string, port int, sender string, recipient string, username string, password string) (*MailAuthHook, error) {
//	err := checkMailHookParams(host, port, sender, recipient)
//	if err != nil {
//		return nil, err
//	}
//
//	return &MailAuthHook{
//		appName:   appName,
//		host:      host,
//		port:      port,
//		sender:    sender,
//		recipient: recipient,
//		username:  username,
//		password:  password,
//	}, nil
//}

// NewStderrHook creates a hook for moving errors to stderr
func NewStderrHook() (*StderrHook, error) {
	return &StderrHook{
		textFormater: new(logrus.TextFormatter),
	}, nil
}

func (es *mailErrStore) saveErrorTime(error string) {
	es.errToTimeMu.Lock()
	defer es.errToTimeMu.Unlock()
	es.errToTime[error] = time.Now()
}

func (es *mailErrStore) markErrAsSent(entry *logrus.Entry) {
	es.saveErrorTime(es.generalErr)
	es.saveErrorTime(entry.Message)
}

func (es *mailErrStore) checkErrorTime(error string, duration time.Duration) bool {
	es.errToTimeMu.RLock()
	defer es.errToTimeMu.RUnlock()
	if errTime, ok := es.errToTime[error]; ok {
		if errTime.Add(duration).After(time.Now()) {
			return false
		}
	}

	return true
}

func (es *mailErrStore) canSendMail(entry *logrus.Entry) bool {
	if !es.checkErrorTime(es.generalErr, time.Minute) {
		return false
	}

	if !es.checkErrorTime(entry.Message, 10*time.Minute) {
		return false
	}

	return true
}

// Fire is called when a log event is fired.
func (hook *MailHook) Fire(entry *logrus.Entry) error {

	// Connect to the remote SMTP server.
	client, err := smtp.Dial(hook.host + ":" + strconv.Itoa(hook.port))
	if err != nil {
		return err
	}

	defer func() { _ = client.Close() }()

	if !errStore.canSendMail(entry) {
		return nil
	}

	if err := client.Mail(hook.sender); err != nil {
		return err
	}

	if err := client.Rcpt(hook.recipient); err != nil {
		return err
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	defer func() { _ = wc.Close() }()

	errStore.markErrAsSent(entry)

	message := createMessage(entry, hook.appName)
	if _, err = message.WriteTo(wc); err != nil {
		return err
	}
	return nil
}

// Fire is called when a log event is fired.
func (hook *MailAuthHook) Fire(entry *logrus.Entry) error {

	if !errStore.canSendMail(entry) {
		return nil
	}

	auth := smtp.PlainAuth("", hook.username, hook.password, hook.host)

	message := createMessage(entry, hook.appName)

	errStore.markErrAsSent(entry)

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	err := smtp.SendMail(
		hook.host+":"+strconv.Itoa(hook.port),
		auth,
		hook.sender,
		[]string{hook.recipient},
		message.Bytes(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (hook *StderrHook) Fire(entry *logrus.Entry) (err error) {
	line, err := hook.textFormater.Format(entry)
	if err == nil {
		_, _ = fmt.Fprintf(os.Stderr, string(line) + string(debug.Stack()))
	}
	return
}

// Levels returns the available logging levels.
func (hook *MailAuthHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.WarnLevel,
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
	}
}

// Levels returns the available logging levels.
func (hook *MailHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.WarnLevel,
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
	}
}

func (hook *StderrHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.WarnLevel,
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
	}
}

func createMessage(entry *logrus.Entry, appname string) *bytes.Buffer {
	subject := appname + " - " + entry.Level.String()
	data, _ := json.MarshalIndent(entry.Data, "", "\t")
	body := "TIME: " + entry.Time.Format("2006-01-02 15:04:05-0700") + "\n" +
		"MESSAGE: " + entry.Message + "\n\n" +
		"DATA: " + string(data) + "\n\n" +
		"STACKTRACE: \n" + string(debug.Stack());

	return bytes.NewBufferString(fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body))
}

func checkMailHookParams(host string, port int, sender string, recipient string) error {

	// Check if server listens on that port.
	conn, err := net.DialTimeout("tcp", host+":"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

	// Validate sender and recipient
	_, err = mail.ParseAddress(sender)
	if err != nil {
		return err
	}
	_, err = mail.ParseAddress(recipient)
	if err != nil {
		return err
	}

	return nil
}
