package commands

import (
	"fmt"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

func logs(router *server.Router, conn pipe.Conn, args ...string) (exitCode int, cmdErr error, err error) {
	var pretty bool
	var logs []logger.Log

	if len(args) != 0 && args[0] == "--pretty" {
		pretty = true
		args = args[1:]
	}

	if len(args) == 0 {
		logs = router.Logger.GetLastNLogs(1000)
	} else {
		switch args[0] {
		case "help":
			err = conn.WriteOutput(logsHelp("help"))
			return
		case "tags":
			logs = logger.LogsMatch(router.Logger.GetLastNLogs(router.Logger.NLogs()), args[1:]...)
		case "tags-any":
			logs = logger.LogsMatchAny(router.Logger.GetLastNLogs(router.Logger.NLogs()), args[1:]...)
		case "level":
			levels := fromStringToLogLevel(args[1:])
			logs = logger.LogsLevelMatch(router.Logger.GetLastNLogs(router.Logger.NLogs()), levels...)
		case "list-tags":
			err = conn.WriteOutput(listTags(router))
			return
		default:
			err = conn.WriteError(logsHelp(args[0]))
			exitCode = 1
			return
		}
	}

	resp := "\n"
	if pretty {
		for _, l := range logs {
			resp += l.FullColored() + "\n"
		}
	} else {
		for _, l := range logs {
			resp += l.Full() + "\n"
		}
	}
	
	err = conn.WriteOutput(resp)
	return
}

func listTags(router *server.Router) string {
	tags := make(map[string]bool)
	for _, l := range router.Logger.GetLastNLogs(router.Logger.NLogs()) {
		for _, t := range l.Tags() {
			tags[t] = true
		}
	}

	res := "Available tags: [ "
	for t := range tags {
		res += fmt.Sprintf("<%s> ", t)
	}
	res += "]"
	return res
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
				 "If --pretty is used as the first argument, the result will be colourful"
}
