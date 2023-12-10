## 开放远程服务器端口
**关闭防火墙**
打开端口也行，直接关了简单
![3a1296541737eb183b39ea7bca791677.png](https://raw.githubusercontent.com/ZhuchaWenjiu/imgback/main/202312102138382.png?token=AKTVCCKFMQKFJYWHE72CDSDFOW7XO)
**添加安全组**
不同的云服务器地方不一样，可以自行百度
将端口添加到安全组
![ecb1ed611ca5390042743de9631d3be5.png](https://raw.githubusercontent.com/ZhuchaWenjiu/imgback/main/202312102138892.png?token=AKTVCCP5WTHGZA5MPU74SLDFOW7YC)
![](https://raw.githubusercontent.com/ZhuchaWenjiu/imgback/main/202312102138892.png?token=AKTVCCP5WTHGZA5MPU74SLDFOW7YC)

## 在远程ECS运行服务端代码
```git
git clone xxxx
cd UdpFileSender/server
go run main.go
```
![e8b55cf316a8fd423dae3f34c302b0ae.png](https://raw.githubusercontent.com/ZhuchaWenjiu/imgback/main/202312102138708.png?token=AKTVCCPCV5KIXCYCSUCJ2FDFOW7YM)

## 本地运行客户端
```git
cd client
go run main.go
```

![f7cd0a38c7816a1fa035ecff337abcec.png](https://raw.githubusercontent.com/ZhuchaWenjiu/imgback/main/202312102138031.png?token=AKTVCCPKK2QTRQDVEY722QTFOW7Y2)

## 代码补充
基于[这里](https://github.com/InferBear/UdpFileSender)
- 添加 md5 校验
- 添加滑动窗口传输