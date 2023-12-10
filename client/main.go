package main

import (
	"UdpFileSender/common"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const FileBlockSize = 1024
const Concurrency = 50
const Windows = int64(5)

var blockFlags []int64
var fileLock sync.Mutex
var wg sync.WaitGroup
var retriedCount = atomic.Int32{}
var totalWrote = int64(0)

func sendRequest(fileReq common.FileRequest, server string) common.FileResponse {
	conn, err := net.Dial("udp", server)
	if err != nil {
		log.Fatalf("Error dialing UDP: %v", err)
	}
	defer conn.Close()

	req, err := json.Marshal(fileReq)
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	_, err = conn.Write(req)
	if err != nil {
		log.Fatalf("Error writing to UDP connection: %v", err)
	}

	var buf [4096]byte
	n, err := conn.Read(buf[0:])
	if err != nil {
		log.Fatalf("Error reading from UDP connection: %v", err)
	}

	var fileResp common.FileResponse
	err = json.Unmarshal(buf[0:n], &fileResp)
	if err != nil {
		log.Fatalf("Error unmarshaling JSON: %v", err)
	}

	return fileResp

}

func getFileSize(server string) int64 {
	log.Println("Requesting file size")
	fileReq := common.FileRequest{Start: math.MaxInt64, End: math.MaxInt64}

	response := make(chan common.FileResponse, 1)

	for {
		timer := time.NewTimer(5 * time.Second)
		go func() {
			data := sendRequest(fileReq, server)
			response <- data
		}()
		select {
		case <-timer.C:
			continue
		case result := <-response:
			return result.Start
		}
	}
}

func requestFileSingalBlock(region, totalBlock int64, file *os.File, server string) {
	if region > totalBlock {
		return
	}
	startFile, endFile := region*FileBlockSize, (region+1)*FileBlockSize
	var pass bool
	var response common.FileResponse
	for !pass { // 添加 md5 校验
		response = requestFileResponse(startFile, endFile, server)
		// get md5
		hasher := md5.New()
		io.WriteString(hasher, string(response.Content))
		md5hash := fmt.Sprintf("%x", hasher.Sum(nil))
		// log.Printf("Md5 judg , %s \n \t\t %s ", md5hash, response.MD5Hash)
		if md5hash == response.MD5Hash {
			pass = true
		}
	}
	blockFlags[region] = int64(1)
	go saveFileBlock(startFile, endFile, totalBlock, file, response.Content)
	log.Printf("Requested file block finished: %d\n", region)
}

func requestFileBlock(region, totalBlock int64, file *os.File, server string) {
	for blockId := region; blockId < totalBlock; blockId += Concurrency { // blockId 起始值为 0 - 49，由于并发 50 个，所以他们每个都是自增 50 块序号是不会有交集的
		startFile, endFile := blockId*FileBlockSize, (blockId+1)*FileBlockSize
		var pass bool
		var response common.FileResponse
		for !pass { // 添加 md5 校验
			response = requestFileResponse(startFile, endFile, server)
			// get md5
			hasher := md5.New()
			io.WriteString(hasher, string(response.Content))
			md5hash := fmt.Sprintf("%x", hasher.Sum(nil))
			// log.Printf("Md5 judg , %s \n \t\t %s ", md5hash, response.MD5Hash)
			if md5hash == response.MD5Hash {
				pass = true
			}
		}
		go saveFileBlock(startFile, endFile, totalBlock, file, response.Content)
		log.Printf("Requested file block finished: %d\n", blockId)
	}
}

// add for md5 judg
func requestFileResponse(start, end int64, server string) common.FileResponse {
	fileReq := common.FileRequest{Start: start, End: end}
	responseChan := make(chan common.FileResponse, 1)
	for {
		timer := time.NewTimer(time.Duration(time.Millisecond * 5000))
		go func() {
			data := sendRequest(fileReq, server)
			responseChan <- data
		}()
		select { // go 的 select 下面的选项任意一个满足条件就执行，如果两个同时满足随机执行
		case res := <-responseChan: // := 猜测类型并赋值，所以res没有定义类型
			return res
		case <-timer.C:
			retriedCount.Add(1)
			log.Printf("File content request timed out, retrying, total Retries(%d), in start = %d", retriedCount, start)
			continue
		}
	}
}

// node
func requestFileContent(start, end int64, server string) []byte {
	fileReq := common.FileRequest{Start: start, End: end}
	responseChan := make(chan common.FileResponse, 1)
	for {
		timer := time.NewTimer(time.Duration(time.Millisecond * 1000))
		go func() {
			data := sendRequest(fileReq, server)
			responseChan <- data
		}()
		select { // go 的 select 下面的选项任意一个满足条件就执行，如果两个同时满足随机执行
		case res := <-responseChan: // := 猜测类型并赋值，所以res没有定义类型
			return res.Content
		case <-timer.C:
			retriedCount.Add(1)
			log.Printf("File content request timed out, retrying, total Retries(%d)", retriedCount)
			continue
		}
	}
}

func saveFileBlock(start, end, totalBlock int64, file *os.File, data []byte) {
	fileLock.Lock()
	defer fileLock.Unlock()
	file.Seek(start, 0)
	file.Write(data)
	totalWrote += 1
	if totalWrote == totalBlock {
		wg.Done()
	}
	log.Printf("Saving file block from %d to %d\n", start, end)
}

func saveFile(savePath string, server string, wd bool) {
	fileSize := getFileSize(server) // 获得要接受的文件大小，普通使用时应该是一个请求，服务器根据请求找到对应文件，然后返回文件大小
	log.Printf("fileSize  %d", int(fileSize))
	file, _ := os.OpenFile(savePath, os.O_RDWR|os.O_CREATE, os.ModePerm)
	totalBlock := fileSize / FileBlockSize // 计算一共有多少文件块
	log.Printf("totalBlocks  %d", int(totalBlock))
	var count = int64(0)
	// 滑动窗口
	if wd {

		blockFlags = make([]int64, totalBlock)
		startWindow := int64(0)
		wg.Add(1)
		for startWindow < totalBlock-Windows && (sumArray(blockFlags) < totalBlock) {
			end := min(totalBlock, startWindow+Windows)
			if sumArray(blockFlags[startWindow:end]) == end-startWindow {
				startWindow += Windows
			}
			for i := startWindow; i < startWindow+Windows; i++ {
				if i < totalBlock {
					if blockFlags[i] == int64(0) {
						count++
						// log.Printf("Count = %d", count)
						go requestFileSingalBlock(i, totalBlock, file, server)
					}
				}
			}

		}
		wg.Wait()
		if fileSize%FileBlockSize != 0 {
			startOffset := fileSize - fileSize%FileBlockSize
			// data := requestFileContent(startOffset, fileSize, server) // 此处也应该添加md5校验
			var pass bool
			var response common.FileResponse
			for !pass { // 添加 md5 校验
				response = requestFileResponse(startOffset, fileSize, server)
				// get md5
				hasher := md5.New()
				io.WriteString(hasher, string(response.Content))
				md5hash := fmt.Sprintf("%x", hasher.Sum(nil))
				if md5hash == response.MD5Hash {
					pass = true
				}
			}
			saveFileBlock(startOffset, fileSize, totalBlock, file, response.Content)
			// saveFileBlock(startOffset, fileSize, totalBlock, file, data)
		}
	} else {
		actualConcurrency := min(totalBlock, Concurrency) // 计数，防止块数超出，每次 concurrency 个块一起运行，每个块中， FileBlocksize 个字节同时获取运行
		wg.Add(1)                                         // 50 个块并发执行
		for i := int64(0); i < actualConcurrency; i++ {
			go requestFileBlock(i, totalBlock, file, server)
		}
		wg.Wait()

		if fileSize%FileBlockSize != 0 {
			startOffset := fileSize - fileSize%FileBlockSize
			// data := requestFileContent(startOffset, fileSize, server) // 此处也应该添加md5校验
			var pass bool
			var response common.FileResponse
			for !pass { // 添加 md5 校验
				response = requestFileResponse(startOffset, fileSize, server)
				// get md5
				hasher := md5.New()
				io.WriteString(hasher, string(response.Content))
				md5hash := fmt.Sprintf("%x", hasher.Sum(nil))
				if md5hash == response.MD5Hash {
					pass = true
				}
			}
			saveFileBlock(startOffset, fileSize, totalBlock, file, response.Content)
			// saveFileBlock(startOffset, fileSize, totalBlock, file, data)
		}
	}

	log.Printf("TotalBlock: %d\n, RetriesCount: (%d) ", totalBlock, retriedCount)
}

func main() {
	saveFile("get.txt", "127.0.0.1:9991", false)
}

func min(a int64, b int64) int64 {
	if a > b {
		return b
	} else {
		return a
	}
}

func sumArray(array []int64) int64 {
	var ans int64
	for i := 0; i < len(array); i++ {
		ans += array[i]
	}
	return ans
}
