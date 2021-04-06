Dead simple file downloader
- [X] parallel download
- [X] resumable
- [X] [progressbar](https://github.com/schollz/progressbar)

### Install
```
git clone https://github.com/mostafa-asg/go-dl
cd go-dl/cmd
go build dl.go
```

### Download a file
```
./dl -u {YOUR_FILE} -n {CONCURRENCY_Level}
./dl -u https://apache.claz.org/zookeeper/zookeeper-3.7.0/apache-zookeeper-3.7.0-bin.tar.gz
```

### Interupt/Pause the download
Ctrl+c

### Resume the download
Use --resume
```
./dl -u https://apache.claz.org/zookeeper/zookeeper-3.7.0/apache-zookeeper-3.7.0-bin.tar.gz --resume
```

### Need more control?
See other options
```
./dl --help
```
