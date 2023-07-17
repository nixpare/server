package commands

import (
	"errors"
	"fmt"

	"github.com/nixpare/logger"
	"github.com/nixpare/server/v2"
)

func logs(router *server.Router, args []string) (resp []byte, err error) {
	if len(args) == 0 {
		return router.Logger.JSON(), nil
	}

	switch args[0] {
	case "tags":
		return logger.LogsToJSON(router.Logger.LogsMatch(args[1:]...)), nil
	case "tags-any":
		return logger.LogsToJSON(router.Logger.LogsMatchAny(args[1:]...)), nil
	case "level":
		levels := fromStringToLogLevel(args[1:])
		return logger.LogsToJSON(router.Logger.LogsLevelMatchAny(levels...)), nil
	default:
		return nil, errors.New(logsHelp(args[0]))
	}
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
	return fmt.Sprintf("invalid sub-command \"%s\" sent: the valid options are:\n", cmd) +
					   "  - tags     [ list of tags to match ]\n" +
					   "  - tags-any [ list of possible tags to match ]\n" +
					   "  - level    [ list of possible levels to match ]"
}
