package mr

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

const (
	MapTask = iota
	ReduceTask
	WaitTask
	NoTask
)

const (
	Idle = iota
	InProgress
	Error
	Completed
)

// constants
const (
	EmptyString = ""
)

type Task struct {
	// Global Configs
	NReduce int

	// Task Configs
	TaskId        int
	Type          int
	FileName      string
	ReducerNumber int
	Status        int
	Attempt       int
}

// globals
var taskCounter = 1
var attemptCounter = 1
var timeout = 10 * time.Second

type Coordinator struct {
	mu                   sync.Mutex
	Tasks                []Task
	IsAllMapTasksDone    bool
	isAllReduceTasksDone bool
}

// Your code here -- RPC handlers for the worker to call.

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

func (c *Coordinator) AskForTask(args AskTaskArgs, reply *AskTaskReply) error {
	if !args {
		return nil
	}
	c.mu.Lock()
	for i, task := range c.Tasks {
		if task.Type == MapTask {
			if task.Status == Idle || task.Status == Error {
				c.Tasks[i].Status = InProgress
				c.Tasks[i].Attempt = attemptCounter
				attemptCounter++

				fillTask(reply, c.Tasks[i])
				fmt.Println("Sending task ", reply)
				break
			}

		} else if task.Type == ReduceTask && c.IsAllMapTasksDone {
			if task.Status == Idle || task.Status == Error {
				c.Tasks[i].Status = InProgress
				c.Tasks[i].Attempt = attemptCounter
				attemptCounter++

				fillTask(reply, c.Tasks[i])
				fmt.Println("Sending task ", reply)
				break
			}
		}
	}

	isAllMapTasksDone := true
	for _, task := range c.Tasks {
		if task.Type == MapTask && task.Status != Completed {
			isAllMapTasksDone = false
			break
		}
	}
	c.IsAllMapTasksDone = isAllMapTasksDone

	isAllReduceTasksDone := true
	for _, task := range c.Tasks {
		if task.Type == ReduceTask && task.Status != Completed {
			isAllReduceTasksDone = false
			break
		}
	}
	c.isAllReduceTasksDone = isAllReduceTasksDone

	c.mu.Unlock()
	go c.waitForTaskCompletion(Task{
		TaskId: reply.TaskId,
		Type:   reply.Type,
	})
	if reply.TaskId == 0 {
		if c.IsAllMapTasksDone && c.isAllReduceTasksDone {
			reply.Type = NoTask
		} else {
			reply.Type = WaitTask
		}
	}

	return nil
}

func (c *Coordinator) waitForTaskCompletion(task Task) {
	if !(task.Type == MapTask || task.Type == ReduceTask) {
		return
	}
	time.Sleep(timeout)
	c.mu.Lock()
	// check if the task is still in progress
	var id int
	for i, t := range c.Tasks {
		if t.TaskId == task.TaskId {
			id = i
			break
		}
	}
	if c.Tasks[id].Status == InProgress {
		fmt.Println("Task ", task, " has timed out")
		c.Tasks[id].Status = Error
	}
	c.mu.Unlock()
}

func fillTask(reply *AskTaskReply, task Task) {
	reply.NReduce = task.NReduce

	reply.TaskId = task.TaskId
	reply.Type = task.Type
	reply.FileName = task.FileName
	reply.ReducerNumber = task.ReducerNumber
	reply.Status = task.Status
	reply.Attempt = task.Attempt
}

func (c *Coordinator) ReportTask(args ReportTaskArgs, reply *ReportTaskReply) error {
	for i, task := range c.Tasks {
		if task.TaskId == args.TaskId {
			c.Tasks[i].Status = args.Status
			reply.IsAcknowledged = true
			return nil
		}
	}
	return nil
}

//
// start a thread that listens for RPCs from worker.go
//
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
//
func (c *Coordinator) Done() bool {
	isDone := c.IsAllMapTasksDone && c.isAllReduceTasksDone
	if isDone {
		fmt.Println("All tasks are done, closing the server")
		time.Sleep(1 * time.Second) // wait for 1 second before closing the server
		return true
	}
	return false
}

//
// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{}

	// need to create the maps tasks
	for _, file := range files {
		c.Tasks = append(c.Tasks, Task{
			NReduce:       nReduce,
			TaskId:        taskCounter,
			Type:          MapTask,
			FileName:      file,
			ReducerNumber: 0,
			Status:        Idle,
			Attempt:       0,
		})
		taskCounter++
	}

	// need to create the reduce tasks
	for i := 0; i < nReduce; i++ {
		c.Tasks = append(c.Tasks, Task{
			NReduce:       nReduce,
			TaskId:        taskCounter,
			Type:          ReduceTask,
			FileName:      EmptyString,
			ReducerNumber: i,
			Status:        Idle,
			Attempt:       0,
		})
		taskCounter++
	}

	fmt.Println("Created tasks ", c.Tasks)
	c.server()
	return &c
}
