package osutils

import "os"

// FileExists 文件是否存在
func FileExists(filepath string) (bool, error) {
	// 获取文件信息
	_, err := os.Stat(filepath)
	// 文件存在
	if err == nil {
		return true, nil
	}
	// 判断文件是否不存在
	if os.IsNotExist(err) {
		return false, nil
	}
	// 返回其他错误信息
	return false, err
}
