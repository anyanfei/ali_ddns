package main

import (
	"io"
	"log"
	"net/http"
	"regexp"
	"time"

	alidns20150109 "github.com/alibabacloud-go/alidns-20150109/v4/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	// ----------以下内容可按需更改，且均为必填项----------
	aliCloudAccessKeyId     = ""  // 通过 https://ram.console.aliyun.com/manage/ak 获取
	aliCloudAccessKeySecret = ""  // 通过 https://ram.console.aliyun.com/manage/ak 获取
	domainURL               = ""  // 你的域名 比如 laghaim.cn
	rr                      = "@" // 主机记录 如果要解析@.exmaple.com，主机记录要填写”@”，而不是空。如果要解析www，就填写www即可
	//主机记录就是域名前缀，常见用法有：
	//www：解析后的域名为www.aliyun.com。
	//@：直接解析主域名 aliyun.com。
	//*：泛解析，匹配其他所有域名 *.aliyun.com。
	//mail：将域名解析为mail.aliyun.com，通常用于解析邮箱服务器。
	//二级域名：如：abc.aliyun.com，填写abc。
	//手机网站：如：m.aliyun.com，填写m。
	//显性URL：不支持泛解析（泛解析：将所有子域名解析到同一地址）
	recordType = "A" // ipv4 参考 https://help.aliyun.com/document_detail/29805.html?spm=a2c4g.29807.0.0.26dc3d95iVibLY

	// ----------以下内容不可更改----------
	aliDnsServer = "alidns.cn-hangzhou.aliyuncs.com" // 不要更改这个，阿里云服务器最快的一个DNS服务器
)

func main() {
	ticker := time.NewTicker(10 * time.Minute)
	err := refreshDDNS() // 首次执行先运行一次
	if err != nil {
		log.Println(err)
	}
	for {
		select {
		case <-ticker.C:
			err = refreshDDNS()
			if err != nil {
				log.Println(err)
			}
		}
	}
}

// CreateClient 创建发起请求的client
func CreateClient(accessKeyId *string, accessKeySecret *string) (result *alidns20150109.Client, err error) {
	config := &openapi.Config{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
	}
	// 访问的域名
	config.Endpoint = tea.String(aliDnsServer)
	result = &alidns20150109.Client{}
	result, err = alidns20150109.NewClient(config)
	return
}

// getRecordIp 获取当前阿里云的记录值
func getRecordIp(client *alidns20150109.Client) (*string, *string, error) {
	result, err := client.DescribeDomainRecords(&alidns20150109.DescribeDomainRecordsRequest{ // 文档出处：https://next.api.aliyun.com/api/Alidns/2015-01-09/UpdateDomainRecord
		DomainName: tea.String(domainURL),
	})
	if err != nil {
		log.Println("获取当前域名IP解析失败", err)
		return nil, nil, err
	}
	records := result.Body.DomainRecords.Record
	var (
		recordIp *string
		recordId *string
	)
	for _, record := range records {
		recordIp = record.Value
		recordId = record.RecordId
	}
	return recordIp, recordId, nil
}

// 执行比对当前ip和dns值，并更新操作
func refreshDDNS() (err error) {
	client, err := CreateClient(tea.String(aliCloudAccessKeyId), tea.String(aliCloudAccessKeySecret))
	if err != nil {
		return err
	}
	recordIp, recordId, err := getRecordIp(client)
	if err != nil {
		return err
	}
	// 获取本机ipv4地址
	realIp := getIpHost()
	if realIp == nil {
		return nil
	}
	if recordIp == nil {
		log.Println("未获取到阿里云Ip解析记录，进行第一次记录")
		err = addDomainFirst(client, realIp)
		if err != nil {
			return err
		}
		log.Println("记录成功，首次记录ip:", *realIp)
		return nil
	}
	if recordIp != nil && *recordIp == *realIp {
		log.Printf("当前公网IP(%s) 与阿里云IP记录值(%s)一致，无需更改\n", *realIp, *recordIp)
		return nil
	} else {
		_, err = client.UpdateDomainRecord(&alidns20150109.UpdateDomainRecordRequest{ // 文档出处：https://next.api.aliyun.com/api/Alidns/2015-01-09/UpdateDomainRecord
			RecordId: recordId,
			RR:       tea.String(rr),
			Type:     tea.String(recordType),
			Value:    realIp,
		})
		if err != nil {
			return err
		}
		recordIp, recordId, err = getRecordIp(client) // 再查一次吧
		if err != nil {
			return err
		}
		log.Printf("记录更新成功，当前的IP地址记录为：%s，记录ID为：%s\n", *recordIp, *recordId)
	}
	return err
}

// addDomainFirst 首次写入
func addDomainFirst(client *alidns20150109.Client, realIp *string) error {
	_, err := client.AddDomainRecord(&alidns20150109.AddDomainRecordRequest{ // 文档出处：https://next.api.aliyun.com/api/Alidns/2015-01-09/AddDomainRecord
		DomainName: tea.String(domainURL),
		RR:         tea.String(rr),
		Type:       tea.String(recordType),
		Value:      realIp,
	})
	if err != nil {
		return err
	}
	return nil
}

// getIpHost 通过访问网站的方式获取ip地址
func getIpHost() *string {
	resp, err := http.Get("https://www.taobao.com/help/getip.php") // 这是PHP写的功能，语言是工具，不是你炫耀的资本，更没有鄙视链一说，只有适合与不适合
	if err != nil {
		log.Println("ip地址查询时出错，error : ", err)
		return nil
	}
	defer resp.Body.Close()
	allByte, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("解析时出错%s", err.Error())
		return nil
	}
	str := string(allByte)
	re := regexp.MustCompile(`ip:"(.*)"`)
	match := re.FindStringSubmatch(str)
	if len(match) != 0 {
		return &match[1]
	} else {
		log.Println("未找到本机公网IP地址")
		return nil
	}
}
