package commands

import (
	"errors"
	"fmt"

	"github.com/nixpare/logger"
	"github.com/nixpare/server/v2"
)

func logs(router *server.Router, args []string) (resp []byte, err error) {
	var pretty bool
	var logs []logger.Log

	if len(args) != 0 && args[0] == "--pretty" {
		pretty = true
		args = args[1:]
	}

	if len(args) == 0 {
		logs = router.Logger.Logs()
	} else {
		switch args[0] {
		case "help":
			resp = []byte(logsHelp("help"))
			return
		case "tags":
			logs = router.Logger.LogsMatch(args[1:]...)
		case "tags-any":
			logs = router.Logger.LogsMatchAny(args[1:]...)
		case "level":
			levels := fromStringToLogLevel(args[1:])
			logs = router.Logger.LogsLevelMatchAny(levels...)
		case "list-tags":
			tags := make(map[string]bool)
			for _, l := range router.Logger.Logs() {
				for _, t := range l.Tags() {
					tags[t] = true
				}
			}
	
			resp = []byte("Available tags: [ ")
			for t := range tags {
				resp = append(resp, []byte(t + " ")...)
			}
			resp = append(resp, []byte("]")...)
			
			return
		default:
			return nil, errors.New(logsHelp(args[0]))
		}
	}

	resp = []byte("\n")
	if pretty {
		for _, l := range logs {
			resp = append(resp, []byte(l.FullColored() + "\n")...)
		}
	} else {
		for _, l := range logs {
			resp = append(resp, []byte(l.Full() + "\n")...)
		}
	}
	
	return
}

func fromStringToLogLevel(levels []string) []logger.LogLevel {
	res := make([]logger.LogLevel, 0, len(levels))
	for _, l := range levels {
		switch l {
		case "blank":
			res = append(res, logger.LOG_LEVEL_BLANK)
		case "info":
			res = append(res, logger.LOG_LEVEL_INFO)
		case "debug":
			res = append(res, logger.LOG_LEVEL_DEBUG)
		case "warning":
			res = append(res, logger.LOG_LEVEL_WARNING)
		case "error":
			res = append(res, logger.LOG_LEVEL_ERROR)
		case "fatal":
			res = append(res, logger.LOG_LEVEL_FATAL)
		}
	}
	return res
}

func logsHelp(cmd string) string {
	var res string
	if cmd == "help" {
		res = "Manage server logs. The valid options are:\n\n"
	} else {
		res = fmt.Sprintf("invalid sub-command \"%s\" sent: the valid options are:\n\n", cmd)
	}
	return res + "    - tags [ tags ... ]       : get all the logs that matches all the tags provided\n" +
				 "    - tags-any [ tags ... ]   : get all the logs that matches at least one tag\n" +
				 "    - level    [ levels ... ] : get all the logs with one of the log severities provided\n" +
				 "    - list-tags               : list of tags currently used by logs\n\n" +
				 "If --pretty is used as the first argument, the result will be colourful\n"
}
