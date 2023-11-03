package commands

import (
	"fmt"
	"time"

	"github.com/nixpare/logger/v2"
)

func watchCmd(sc *ServerConn, args ...string) (exitCode int, err error) {
	printLog := func(l logger.Log) error {
		return sc.WriteOutput(l.Full())
	}
	
	if len(args) > 0 && args[len(args)-1] == "--pretty" {
		args = args[:len(args)-1]
		printLog = func(l logger.Log) error {
			return sc.WriteOutput(l.FullColored())
		}
	}

	logSelector := func(l logger.Log) bool {
		return true
	}

	if len(args) == 0 {
		return watchLoop(sc, printLog, logSelector)
	}

	switch args[0] {
	case "tags":
		logSelector = func(l logger.Log) bool {
			return l.Match(args[1:]...)
		}
	case "tags-any":
		logSelector = func(l logger.Log) bool {
			return l.MatchAny(args[1:]...)
		}
	case "level":
		levels := fromStringToLogLevel(args[1:])
		logSelector = func(l logger.Log) bool {
			return l.LevelMatchAny(levels...)
		}
	case "help":
		return 0, sc.WriteOutput(watchHelp(args[0]))
	default:
		return 1, sc.WriteError(serverHelp(args[0]))
	}

	return watchLoop(sc, printLog, logSelector)
}

func watchLoop(sc *ServerConn, printLog func(l logger.Log) error, logSelector func(l logger.Log) bool) (int, error) {
	lastSent := sc.Router.Logger.NLogs()
	for _, l := range sc.Router.Logger.GetLogs(0, lastSent) {
		if !logSelector(l) {
			continue
		}

		err := printLog(l)
		if err != nil {
			return 1, err
		}
	}

	type exitRes struct {
		exitCode int
		err      error
	}

	ticker := time.NewTicker(time.Second)
	exitC := make(chan exitRes)
	defer close(exitC)
	stopWatching := make(chan exitRes)
	defer close(stopWatching)

	go func() {
		for {
			select {
			case <-ticker.C:
				logs := sc.Router.Logger.GetLastNLogs(sc.Router.Logger.NLogs() - lastSent)
				lastSent += len(logs)

				for _, l := range logs {
					if !logSelector(l) {
						continue
					}

					err := printLog(l)
					if err != nil {
						exitC <- exitRes{1, err}
					}
				}
			case r := <-stopWatching:
				ticker.Stop()
				exitC <- r
				return
			}
		}
	}()

	go func() {
		for {
			msg, err := sc.ReadMessage()
			if err != nil {
				stopWatching <- exitRes{1, err}
				return
			}

			if msg == "q" {
				stopWatching <- exitRes{0, nil}
				return
			}
		}
	}()

	res := <-exitC
	return res.exitCode, res.err
}

func watchHelp(cmd string) string {
	var res string
	if cmd == "help" {
		res = "Attach the standard output and error to the server Logger. End the execution with a CTRL-C or by sending a \"q\".\nThe other valid options are:\n\n"
	} else {
		res = fmt.Sprintf("Invalid sub-command \"%s\" sent: the valid options are:\n\n", cmd)
	}
	return res + "    - tags     [ tags ... ]   : outputs only the logs that matches all the tags provided\n" +
				 "    - tags-any [ tags ... ]   : outputs only the logs that matches at least one tag\n" +
				 "    - level    [ levels ... ] : outputs only the logs with one of the log severities provided\n" +
				 "    - help                    : prints out the help message\n\n" +
		"If --pretty is used as the last argument, the result will be colourful\n"
}
