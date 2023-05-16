package server

import (
	"fmt"
	"log"

	"github.com/nixpare/logger"
	"github.com/nixpare/process"
	"github.com/yookoala/gofast"
)

type PHPProcessor struct {
	connFactory gofast.ConnFactory
	process     *process.Process
	logger      *logger.Logger
}

func CreatePHPProcessor(srv *Server, port int) (php *PHPProcessor, err error) {
	php = new(PHPProcessor)
	
	php.logger = srv.Logger.Clone(nil, "php")
	php.process, err = process.NewProcess("", "php-cgi", process.ParseCommandArgs(fmt.Sprintf("-b %d", port))...)
	if err != nil {
		return
	}

	php.connFactory = gofast.SimpleConnFactory("tcp", fmt.Sprintf(":%d", port))
	return
}

func (php *PHPProcessor) Start() error {
	err := php.process.Start(nil, nil, nil)
	if err != nil {
		return err
	}

	go func() {
		exitStatus := php.process.Wait()
		if (exitStatus.ExitCode != 0 || exitStatus.ExitError != nil) && exitStatus.ExitCode != 0xc000013a {
			php.logger.Print(logger.LOG_LEVEL_ERROR, exitStatus)
		}
	}()

	return nil
}

func (php *PHPProcessor) Stop() error {
	if php.process.IsRunning() {
		err := php.process.Stop()
		if err != nil {
			return err
		}
	}

	exitStatus := php.process.Wait()
	if (exitStatus.ExitCode != 0 || exitStatus.ExitError != nil) && exitStatus.ExitCode != 0xc000013a {
		return fmt.Errorf("exit status (code %d): %v", exitStatus.ExitCode, exitStatus.ExitError)
	}
	return nil
}

func (route *Route) ServePHP(php *PHPProcessor) {
	h := gofast.NewHandler(
		gofast.NewFileEndpoint(route.Website.Dir + route.RequestURI)(gofast.BasicSession),
		gofast.SimpleClientFactory(php.connFactory),
	)

	phpLogger := route.Logger.Clone(nil, "php")

	h.SetLogger(log.New(phpLogger, "PHP", 0))
	h.ServeHTTP(route.W, route.R)
}
