package commands

import (
	"fmt"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
)

func logCmd(sc *ServerConn, args ...string) (int, error) {
	var pretty bool
	var logs []logger.Log

	if len(args) > 0 && args[len(args)-1] == "--pretty" {
		pretty = true
		args = args[:len(args)-1]
	}

	if len(args) == 0 {
		logs = sc.Router.Logger.GetLastNLogs(1000)
	} else {
		switch args[0] {
		case "help":
			return 0, sc.WriteOutput(logHelp("help"))
		case "all":
			logs = sc.Router.Logger.GetLastNLogs(sc.Router.Logger.NLogs())
		case "tags":
			logs = logger.LogsMatch(
				sc.Router.Logger.GetLastNLogs(sc.Router.Logger.NLogs()),
				args[1:]...,
			)
		case "tags-any":
			logs = logger.LogsMatchAny(
				sc.Router.Logger.GetLastNLogs(sc.Router.Logger.NLogs()),
				args[1:]...,
			)
		case "level":
			levels := fromStringToLogLevel(args[1:])
			logs = logger.LogsLevelMatch(
				sc.Router.Logger.GetLastNLogs(sc.Router.Logger.NLogs()),
				levels...,
			)
		case "range":
			if len(args) < 2 {
				return 1, sc.WriteError("Not enough arguments")
			}

			var start, end int
			_, err := fmt.Sscanf(args[1], "%d:%d", &start, &end)
			if err != nil {
				return 1, sc.WriteError(err.Error())
			}

			logs = sc.Router.Logger.GetLogs(start, end)
		case "list-tags":
			return 0, sc.WriteOutput(listTags(sc.Router))
		default:
			return 1, sc.WriteError(logHelp(args[0]))
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

	return 0, sc.WriteOutput(resp)
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

func logHelp(cmd string) string {
	var res string
	if cmd == "help" {
		res = "Gather server logs. By default, it returns the last 1000 logs, if available.\nThe other valid options are:\n\n"
	} else {
		res = fmt.Sprintf("Invalid sub-command \"%s\" sent: the valid options are:\n\n", cmd)
	}
	return res + "    - all                     : get all the logs available\n" +
				 "    - tags     [ tags ... ]   : get all the logs that matches all the tags provided\n" +
				 "    - tags-any [ tags ... ]   : get all the logs that matches at least one tag\n" +
				 "    - level    [ levels ... ] : get all the logs with one of the log severities provided\n" +
				 "    - range    <start>:<end>  : get all the logs with the index <start> <= i < <end>\n" +
				 "    - list-tags               : list of tags currently used by logs\n\n" +
				 "    - help                    : prints out the help message\n\n" +
				 "If --pretty is used as the last argument, the result will be colourful\n"
}
