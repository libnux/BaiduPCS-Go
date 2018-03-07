package baidupcs

import (
	"bytes"
	"fmt"
	"github.com/iikira/BaiduPCS-Go/pcsutil"
	"github.com/iikira/BaiduPCS-Go/pcsverbose"
	"github.com/iikira/BaiduPCS-Go/requester/multipartreader"
	"github.com/json-iterator/go"
)

// FileDirectory 文件或目录的详细信息
type FileDirectory struct {
	FsID        int64  // fs_id
	Path        string // 路径
	Filename    string // 文件名 或 目录名
	Ctime       int64  // 创建日期
	Mtime       int64  // 修改日期
	MD5         string // md5 值
	Size        int64  // 文件大小 (目录为0)
	Isdir       bool   // 是否为目录
	Ifhassubdir bool   // 是否含有子目录 (只对目录有效)

	Parent   *FileDirectory    // 父目录信息
	Children FileDirectoryList // 子目录信息
}

// FileDirectoryList FileDirectory 的 指针数组
type FileDirectoryList []*FileDirectory

// fdJSON 用于解析远程JSON数据
type fdJSON struct {
	FsID           int64  `json:"fs_id"`           // fs_id
	Path           string `json:"path"`            // 路径
	Filename       string `json:"server_filename"` // 文件名 或 目录名
	Ctime          int64  `json:"ctime"`           // 创建日期
	Mtime          int64  `json:"mtime"`           // 修改日期
	MD5            string `json:"md5"`             // md5 值
	Size           int64  `json:"size"`            // 文件大小 (目录为0)
	IsdirInt       int    `json:"isdir"`
	IfhassubdirInt int    `json:"ifhassubdir"`
}

// convert 将解析的远程JSON数据, 转换为 *FileDirectory
func (fj *fdJSON) convert() *FileDirectory {
	return &FileDirectory{
		FsID:        fj.FsID,
		Path:        fj.Path,
		Filename:    fj.Filename,
		Ctime:       fj.Ctime,
		Mtime:       fj.Mtime,
		MD5:         fj.MD5,
		Size:        fj.Size,
		Isdir:       pcsutil.IntToBool(fj.IsdirInt),
		Ifhassubdir: pcsutil.IntToBool(fj.IfhassubdirInt),
	}
}

type fdData struct {
	*ErrInfo
	List []*fdJSON `json:"list"`
}

// FilesDirectoriesMeta 获取单个文件/目录的元信息
func (p *PCSApi) FilesDirectoriesMeta(path string) (data *FileDirectory, err error) {
	operation := "获取单个文件/目录的元信息"

	if path == "" {
		path = "/"
	}

	fds, err := p.FilesDirectoriesBatchMeta(path)
	if err != nil {
		return nil, fmt.Errorf("%s, 路径: %s", err, path)
	}

	if len(fds) != 1 {
		return nil, fmt.Errorf("%s发生错误, 未知返回数据, 路径: %s", operation, path)
	}

	return fds[0], nil
}

// FilesDirectoriesBatchMeta 获取多个文件/目录的元信息
func (p *PCSApi) FilesDirectoriesBatchMeta(paths ...string) (data FileDirectoryList, err error) {
	operation := "获取文件/目录的元信息"

	type listStr struct {
		Path string `json:"path"`
	}

	type postStr struct {
		List []listStr `json:"list"`
	}

	// 数据处理
	post := &postStr{
		List: make([]listStr, len(paths)),
	}

	for k := range paths {
		post.List[k].Path = paths[k]
	}

	sendData, err := jsoniter.Marshal(post)
	if err != nil {
		panic(operation + ", json 数据构造失败, " + err.Error())
	}

	p.setAPI("file", "meta")

	// 表单上传
	mr := multipartreader.NewMultipartReader()
	mr.AddFormFeild("param", bytes.NewReader(sendData))

	resp, err := p.client.Req("POST", p.url.String(), mr, nil)
	if err != nil {
		return nil, fmt.Errorf("%s, 网络错误, %s", operation, err)
	}

	defer resp.Body.Close()

	// 服务器返回数据进行处理
	jsonData := &fdData{
		ErrInfo: NewErrorInfo(operation),
	}

	d := jsoniter.NewDecoder(resp.Body)
	err = d.Decode(jsonData)
	if err != nil {
		return nil, fmt.Errorf("%s, json 数据解析失败, %s", operation, err)
	}

	// 错误处理
	errCode, _ := jsonData.ErrInfo.FindErr()
	if errCode != 0 {
		return nil, jsonData.ErrInfo
	}

	// 结果处理
	data = make(FileDirectoryList, len(jsonData.List))
	for k := range jsonData.List {
		data[k] = jsonData.List[k].convert()
	}

	return
}

// FilesDirectoriesList 获取目录下的文件和目录列表, 可选是否递归
func (p *PCSApi) FilesDirectoriesList(path string, recurse bool) (data FileDirectoryList, err error) {
	operation := "获取目录下的文件列表"

	if path == "" {
		path = "/"
	}

	p.setAPI("file", "list", map[string]string{
		"path":  path,
		"by":    "name",
		"order": "asc", // 升序
		"limit": "0-2147483647",
	})

	resp, err := p.client.Req("GET", p.url.String(), nil, nil)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	jsonData := &fdData{
		ErrInfo: NewErrorInfo(operation),
	}

	d := jsoniter.NewDecoder(resp.Body)
	err = d.Decode(jsonData)
	if err != nil {
		return nil, fmt.Errorf("%s, json 数据解析失败, %s", operation, err)
	}

	// 错误处理
	errCode, _ := jsonData.ErrInfo.FindErr()
	if errCode != 0 {
		return nil, fmt.Errorf("%s, 路径: %s", jsonData.ErrInfo, path)
	}

	data = make(FileDirectoryList, len(jsonData.List))
	for k := range jsonData.List {
		data[k] = jsonData.List[k].convert()

		// 递归获取子目录信息
		if recurse && data[k].Isdir {
			data[k].Children, err = p.FilesDirectoriesList(data[k].Path, recurse)
			if err != nil {
				pcsverbose.Verboseln(err)
			}
		}
	}

	return
}

func (f *FileDirectory) String() string {
	if f.Isdir {
		return fmt.Sprintf("类型: 目录 \n目录名称: %s \n目录路径: %s \nfs_id: %d \n创建日期: %s \n修改日期: %s \n是否含有子目录: %t\n",
			f.Filename,
			f.Path,
			f.FsID,
			pcsutil.FormatTime(f.Ctime),
			pcsutil.FormatTime(f.Mtime),
			f.Ifhassubdir,
		)
	}

	return fmt.Sprintf("类型: 文件 \n文件名: %s \n文件路径: %s \n文件大小: %d, (%s) \nmd5: %s \nfs_id: %d \n创建日期: %s \n修改日期: %s \n",
		f.Filename,
		f.Path,
		f.Size, pcsutil.ConvertFileSize(f.Size),
		f.MD5,
		f.FsID,
		pcsutil.FormatTime(f.Ctime),
		pcsutil.FormatTime(f.Mtime),
	)
}

// TotalSize 获取目录下文件的总大小
func (fl FileDirectoryList) TotalSize() int64 {
	var size int64
	for k := range fl {
		if fl[k] == nil {
			continue
		}

		size += fl[k].Size

		// 递归获取
		if fl[k].Children != nil {
			size += fl[k].Children.TotalSize()
		}
	}
	return size
}

// Count 获取文件总数和目录总数
func (fl FileDirectoryList) Count() (fileN, directoryN int64) {
	for k := range fl {
		if fl[k] == nil {
			continue
		}

		if fl[k].Isdir {
			directoryN++
		} else {
			fileN++
		}

		// 递归获取
		if fl[k].Children != nil {
			fN, dN := fl[k].Children.Count()
			fileN += fN
			directoryN += dN
		}
	}
	return
}

// AllFilePaths 返回所有的网盘路径, 包括子目录
func (fl FileDirectoryList) AllFilePaths() (pcspaths []string) {
	fN, dN := fl.Count()
	pcspaths = make([]string, fN+dN)
	for k := range fl {
		if fl[k] == nil {
			continue
		}

		pcspaths = append(pcspaths, fl[k].Path)

		if fl[k].Children != nil {
			pcspaths = append(pcspaths, fl[k].Children.AllFilePaths()...)
		}
	}
	return
}
