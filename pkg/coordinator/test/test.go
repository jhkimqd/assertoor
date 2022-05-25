package test

import (
	"context"
	"fmt"
	"time"

	"github.com/samcm/sync-test-coordinator/pkg/coordinator/task"
	"github.com/sirupsen/logrus"
)

type Runnable interface {
	Validate() error
	Run(ctx context.Context) error
	Name() string
	Percent() float64
	Tasks() []task.Runnable
	ActiveTask() task.Runnable
}

type Test struct {
	name  string
	tasks []task.Runnable
	log   logrus.FieldLogger

	activeTask task.Runnable
	currIndex  int
}

var _ Runnable = (*Test)(nil)

func AvailableTasks() task.MapOfRunnableInfo {
	return task.AvailableTasks()
}

func CreateTest(ctx context.Context, log logrus.FieldLogger, executionURL, consensusURL string, config Config) (Runnable, error) {
	runnable := &Test{
		name:      config.Name,
		tasks:     []task.Runnable{},
		log:       log.WithField("test", config.Name),
		currIndex: 1,
	}

	for _, taskConfig := range config.Tasks {
		t, err := task.NewRunnableByName(ctx, log, executionURL, consensusURL, taskConfig.Name, taskConfig.Config)
		if err != nil {
			return nil, err
		}

		log.WithField("config", t.Config()).WithField("task", t.Name()).Info("created task")

		runnable.tasks = append(runnable.tasks, t)
	}

	return runnable, nil
}

func (t *Test) Name() string {
	return t.name
}

func (t *Test) Validate() error {
	for _, task := range t.tasks {
		if err := task.ValidateConfig(); err != nil {
			return fmt.Errorf("task %s config validation failed: %s", task.Name(), err)
		}
	}

	if len(t.tasks) == 0 {
		return fmt.Errorf("test %s has no tasks", t.name)
	}

	return nil
}

func (t *Test) Run(ctx context.Context) error {
	for _, task := range t.tasks {
		t.log.WithField("task", task.Name()).Info("starting task")

		t.activeTask = task

		if err := t.runTask(ctx, task); err != nil {
			return err
		}

		t.currIndex++

		t.log.WithField("task", task.Name()).Info("task completed!")
	}

	t.log.Info("test completed!")

	return nil
}

func (t *Test) Percent() float64 {
	return float64(t.currIndex) / float64(len(t.tasks))
}

func (t *Test) Tasks() []task.Runnable {
	return t.tasks
}

func (t *Test) ActiveTask() task.Runnable {
	return t.activeTask
}

func (t *Test) runTask(ctx context.Context, ta task.Runnable) error {
	if err := ta.Start(ctx); err != nil {
		return err
	}

	if complete := t.tickTask(ctx, ta); complete {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(ta.PollingInterval()):
			if complete := t.tickTask(ctx, ta); complete {
				return nil
			}
		}
	}
}

func (t *Test) tickTask(ctx context.Context, ta task.Runnable) bool {
	log := t.log.WithField("task", ta.Name())

	log.Info("checking task for completion")

	complete, err := ta.IsComplete(ctx)

	log.WithFields(logrus.Fields{
		"complete": complete,
		"err":      err,
	}).Info("task status check")

	if err != nil {
		return false
	}

	if !complete {
		return false
	}

	t.log.Info("task is complete")

	return true
}