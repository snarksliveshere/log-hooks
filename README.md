# Logger hooks for Logrus

##Methods:
* `func UsefulSetupLogrus(
   	log *logrus.Logger,
    mailHostPort string,
   	format string,
   	level string,
   	appName string,
   	sender string,
   	recipient string
   ) error` 
   * set output format to stdout [text|json]
   * set verbosity [panic|fatal|error|warn|info|debug|trace]
   * sending errors to emails [panic|fatal|error|warn]
   * sending logs to stdout [info|debug|trace|panic|fatal|error|warn] and errors to stderr [panic|fatal|error|warn] 


##Usage
```go
package main
import  (
    "gitlab.mobio.ru/go-packages/log-hooks"
    "github.com/sirupsen/logrus"
)
logger := logrus.New()
err := log_hooks.UsefulSetupLogrus(
    logger,
    "json",
    "debug",
    "My microservice",
	"sender@domain",
	"recipient@domain",
)
if err != nil {
    panic(err)
}
```

##Dependencies
* `github.com/sirupsen/logrus`  - Logger with hooks
