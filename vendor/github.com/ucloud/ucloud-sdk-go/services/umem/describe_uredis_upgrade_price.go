//Code is generated by ucloud code generator, don't modify it by hand, it will cause undefined behaviors.
//go:generate ucloud-gen-go-api UMem DescribeURedisUpgradePrice

package umem

import (
	"github.com/ucloud/ucloud-sdk-go/ucloud/request"
	"github.com/ucloud/ucloud-sdk-go/ucloud/response"
)

// DescribeURedisUpgradePriceRequest is request schema for DescribeURedisUpgradePrice action
type DescribeURedisUpgradePriceRequest struct {
	request.CommonBase

	// 可用区。参见 [可用区列表](../summary/regionlist.html)
	Zone *string `required:"true"`

	// 购买uredis大小,单位:GB,范围是[1-32]
	Size *int `required:"true"`

	// 要升级的空间的GroupId,请参考DescribeURedisGroup接口
	GroupId *string `required:"true"`

	//
	Type *string `required:"true"`
}

// DescribeURedisUpgradePriceResponse is response schema for DescribeURedisUpgradePrice action
type DescribeURedisUpgradePriceResponse struct {
	response.CommonBase

	// 扩容差价，单位: 元，保留小数点后两位有效数字
	Price float64
}

// NewDescribeURedisUpgradePriceRequest will create request of DescribeURedisUpgradePrice action.
func (c *UMemClient) NewDescribeURedisUpgradePriceRequest() *DescribeURedisUpgradePriceRequest {
	req := &DescribeURedisUpgradePriceRequest{}

	// setup request with client config
	c.client.SetupRequest(req)

	// setup retryable with default retry policy (retry for non-create action and common error)
	req.SetRetryable(true)
	return req
}

// DescribeURedisUpgradePrice - 获取uredis升级价格信息
func (c *UMemClient) DescribeURedisUpgradePrice(req *DescribeURedisUpgradePriceRequest) (*DescribeURedisUpgradePriceResponse, error) {
	var err error
	var res DescribeURedisUpgradePriceResponse

	err = c.client.InvokeAction("DescribeURedisUpgradePrice", req, &res)
	if err != nil {
		return &res, err
	}

	return &res, nil
}
