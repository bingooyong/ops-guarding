package timer

import (
	"ops-guarding/pkg/appframe/logger"
	"ops-guarding/pkg/models"
	"time"
)

func CleanLong() {
	d := time.Duration(24) * time.Hour
	for {
		cleanLongTask()
		time.Sleep(d)
	}
}

func cleanLongTask() {
	ids, err := models.LongTaskIds()
	if err != nil {
		logger.Errorf("LongTaskIds: %v", err)
		return
	}

	if ids == nil {
		return
	}

	count := len(ids)
	for i := 0; i < count; i++ {
		action := models.TaskAction{Id: ids[i]}
		err = action.Update("cancel")
		if err != nil {
			logger.Errorf("cannot cancel long task[%d]: %v", ids[i], err)
		}
	}
}
