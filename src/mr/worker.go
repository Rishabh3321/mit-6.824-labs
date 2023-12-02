package mr

import (
	"fmt"
	"hash/fnv"
	"log"
	"net/rpc"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

var noMoreTasks = false

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.

	for !noMoreTasks {
		task := AskTask()
		if task.Type == MapTask {
			content, err := os.ReadFile(task.FileName)
			if err != nil {
				fmt.Println("Error reading file ", task)
				log.Fatal(err)
				ReportTask(Task{
					TaskId: task.TaskId,
					Status: Error,
				})
			}
			arr := mapf(task.FileName, string(content))
			arrOfArr := make([][]KeyValue, task.NReduce)
			for _, kv := range arr {
				index := ihash(kv.Key) % task.NReduce
				arrOfArr[index] = append(arrOfArr[index], kv)
			}

			// sort the array
			for i := 0; i < task.NReduce; i++ {
				arr := arrOfArr[i]
				sort.Slice(arr, func(i, j int) bool {
					return arr[i].Key < arr[j].Key
				})
				arrOfArr[i] = arr
			}

			// write the files
			for i := 0; i < task.NReduce; i++ {
				fileName := fmt.Sprintf("mr-temp-%d-%d", i, task.TaskId)
				file, err := os.Create(fileName)
				if err != nil {
					log.Fatal(err)
					ReportTask(Task{
						TaskId: task.TaskId,
						Status: Error,
					})
				}
				for _, kv := range arrOfArr[i] {
					fmt.Fprintf(file, "%v %v\n", kv.Key, kv.Value)
				}
				file.Close()
			}

			task.Status = Completed
			ReportTask(Task{
				TaskId: task.TaskId,
				Status: Completed,
			})
		} else if task.Type == ReduceTask {

			// identify the files to read
			var files []string
			allContent, err := os.ReadDir(".")
			if err != nil {
				log.Fatal(err)
				ReportTask(Task{
					TaskId: task.TaskId,
					Status: Error,
				})
			}

			for _, file := range allContent {
				if file.IsDir() {
					continue
				}
				fileName := file.Name()
				prefix := "mr-temp-" + strconv.Itoa(task.ReducerNumber)
				if strings.Contains(fileName, prefix) {
					files = append(files, fileName)
				}
			}

			fmt.Println("Files to read ", files)

			// read the files
			data := make([]KeyValue, 0)
			for _, fileName := range files {
				content, err := os.ReadFile(fileName)
				if err != nil {
					log.Fatal(err)
					ReportTask(Task{
						TaskId: task.TaskId,
						Status: Error,
					})
				}

				// read the file content line by line
				lines := strings.Split(string(content), "\n")
				for _, line := range lines {
					if len(line) == 0 {
						continue
					}
					arr := strings.Split(line, " ")
					data = append(data, KeyValue{
						Key:   arr[0],
						Value: arr[1],
					})
				}
			}

			sort.Slice(data, func(i, j int) bool {
				return data[i].Key < data[j].Key
			})

			// call the reduce function
			mapOfKeys := make(map[string]string)
			for i := 0; i < len(data); i++ {
				j := i
				for j < len(data) && data[j].Key == data[i].Key {
					j++
				}
				values := make([]string, 0)
				for k := i; k < j; k++ {
					values = append(values, data[k].Value)
				}
				output := reducef(data[i].Key, values)
				mapOfKeys[data[i].Key] = output
				i = j - 1
			}

			// write the output file
			fileName := fmt.Sprintf("mr-out-%d", task.Attempt)
			file, err := os.Create(fileName)
			if err != nil {
				log.Fatal(err)
				ReportTask(Task{
					TaskId: task.TaskId,
					Status: Error,
				})
			}
			for key, value := range mapOfKeys {
				fmt.Fprintf(file, "%v %v\n", key, value)
			}
			file.Close()

			// delete the temp files
			for _, fileName := range files {
				err := os.Remove(fileName)
				if err != nil {
					log.Fatal(err)
					ReportTask(Task{
						TaskId: task.TaskId,
						Status: Error,
					})
				}
			}

			task.Status = Completed
			ReportTask(Task{
				TaskId: task.TaskId,
				Status: Completed,
			})

		} else if task.Type == WaitTask {
			fmt.Println("Waiting for 1 second")
			time.Sleep(1 * time.Second)
		} else if task.Type == NoTask {
			noMoreTasks = true
		}
	}

	// uncomment to send the Example RPC to the coordinator.
	// CallExample()
}

//
// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

func AskTask() Task {
	args := AskTaskArgs(true)
	reply := AskTaskReply{}
	ok := call("Coordinator.AskForTask", &args, &reply)
	if ok {
		return Task(reply)
	} else {
		return Task{}
	}
}

func ReportTask(task Task) {
	args := ReportTaskArgs{
		TaskId: task.TaskId,
		Status: task.Status,
	}
	reply := ReportTaskReply{}
	ok := call("Coordinator.ReportTask", &args, &reply)
	if ok {
		fmt.Printf("Reported Task %v \n", task)
	} else {
		fmt.Printf("Failed to report Task %v \n", task)
	}
}

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
