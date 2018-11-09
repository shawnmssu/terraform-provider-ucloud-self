package ucloud

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/ucloud/ucloud-sdk-go/ucloud"
)

func resourceUCloudDBParamGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceUCloudDBParamGroupCreate,
		Read:   resourceUCloudDBParamGroupRead,
		Delete: resourceUCloudDBParamGroupDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"availability_zone": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"name": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateDBInstanceName,
			},

			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"src_group_id": &schema.Schema{
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
			},

			"engine": &schema.Schema{
				Type:         schema.TypeString,
				ValidateFunc: validateStringInChoices([]string{"mysql", "percona", "postgresql"}),
				ForceNew:     true,
				Required:     true,
			},

			"engine_version": &schema.Schema{
				Type:         schema.TypeString,
				ValidateFunc: validateStringInChoices([]string{"5.1", "5.5", "5.6", "5.7", "9.4", "9.6", "10.4"}),
				ForceNew:     true,
				Required:     true,
			},

			"region_flag": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: true,
			},

			"parameter_input": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"key": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"value": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
				Set: resourceUCloudDBParameterHash,
			},

			"parameter_output": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"key": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"value": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func resourceUCloudDBParamGroupCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*UCloudClient)
	conn := client.udbconn

	zone := d.Get("availability_zone").(string)
	engine := d.Get("engine").(string)
	engineVersion := d.Get("engine_version").(string)
	dbType := strings.Join([]string{engine, engineVersion}, "-")
	srcGroupId, err := strconv.Atoi(d.Get("src_group_id").(string))
	if err != nil {
		return err
	}
	dbPg, err := client.describeDBParamGroupById(d.Get("src_group_id").(string))
	if err != nil {
		return fmt.Errorf("do %s failed in create param group, %s", "DescribeUDBParamGroup", err)
	}

	if dbPg.DBTypeId != dbType {
		return fmt.Errorf("\"src_group_id\" is invalid, the corresponding type of database should be %s, got %s", dbType, dbPg.DBTypeId)
	}

	_, ok := d.GetOk("parameter_input")
	if !ok {
		req := conn.NewCreateUDBParamGroupRequest()
		req.GroupName = ucloud.String(d.Get("name").(string))
		req.Zone = ucloud.String(zone)
		req.DBTypeId = ucloud.String(dbType)
		req.SrcGroupId = ucloud.Int(srcGroupId)

		if val, ok := d.GetOk("description"); ok {
			req.Description = ucloud.String(val.(string))
		}

		if val, ok := d.GetOk("region_flag"); ok {
			req.RegionFlag = ucloud.Bool(val.(bool))
		}

		resp, err := conn.CreateUDBParamGroup(req)
		if err != nil {
			return fmt.Errorf("error in create db param group, %s", err)
		}

		d.SetId(strconv.Itoa(resp.GroupId))
	} else {
		paramInput := d.Get("parameter_input").(*schema.Set)
		memberSet := dbPg.ParamMember
		member := make(map[string]string)
		for _, item := range memberSet {
			member[item.Key] = item.Value
		}

		for _, item := range paramInput.List() {
			p := item.(map[string]interface{})
			k := p["key"].(string)
			v := p["value"].(string)
			if _, ok := member[k]; ok {
				member[k] = v
			} else {
				return fmt.Errorf("the corresponding key %s of \"parameter_input\" is invalid", k)
			}
		}

		content := []string{}
		if dbPg.DBTypeId == "mysql" || dbPg.DBTypeId == "percona" {
			content = append(content, "[mysqld]")
		} else if dbPg.DBTypeId == "postgresql" {
			content = append(content, "postgresql")
		}

		for key, value := range member {
			content = append(content, fmt.Sprintf("%s = %s", key, value))
		}

		contented := base64.StdEncoding.EncodeToString([]byte(strings.Join(content, "\n")))

		upReq := conn.NewUploadUDBParamGroupRequest()
		upReq.DBTypeId = ucloud.String(strings.Join([]string{engine, engineVersion}, "-"))
		upReq.GroupName = ucloud.String(d.Get("name").(string))
		if val, ok := d.GetOk("description"); ok {
			upReq.Description = ucloud.String(val.(string))
		}
		upReq.Content = ucloud.String(contented)
		if val, ok := d.GetOk("region_flag"); ok {
			upReq.RegionFlag = ucloud.Bool(val.(bool))
		}

		upResp, err := conn.UploadUDBParamGroup(upReq)
		if err != nil {
			return err
		}

		d.SetId(strconv.Itoa(upResp.GroupId))
	}

	return resourceUCloudDBParamGroupRead(d, meta)
}

func resourceUCloudDBParamGroupRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*UCloudClient)

	dbPg, err := client.describeDBParamGroupById(d.Id())
	if err != nil {
		if isNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("do %s failed in read param group, %s", "DescribeUDBParamGroup", err)
	}

	arr := strings.Split(dbPg.DBTypeId, "-")
	d.Set("name", dbPg.GroupName)
	d.Set("engine", arr[0])
	d.Set("engine_version", arr[1])
	d.Set("description", dbPg.Description)

	parameterOutput := []map[string]interface{}{}
	for _, item := range dbPg.ParamMember {
		parameterOutput = append(parameterOutput, map[string]interface{}{
			"key":   item.Key,
			"value": item.Value,
		})
	}
	d.Set("parameter_output", parameterOutput)

	return nil
}

func resourceUCloudDBParamGroupDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*UCloudClient)
	conn := client.udbconn

	req := conn.NewDeleteUDBParamGroupRequest()
	groupId, err := strconv.Atoi(d.Id())
	if err != nil {
		return err
	}

	req.GroupId = ucloud.Int(groupId)
	if val, ok := d.GetOk("region_flag"); ok {
		req.RegionFlag = ucloud.Bool(val.(bool))
	}

	return resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, err := client.describeDBParamGroupById(d.Id())
		if err != nil {
			if isNotFoundError(err) {
				return nil
			}
			return resource.NonRetryableError(err)
		}

		if _, err := conn.DeleteUDBParamGroup(req); err != nil {
			return resource.NonRetryableError(fmt.Errorf("error in delete db param group %s, %s", d.Id(), err))
		}

		if _, err := client.describeDBParamGroupById(d.Id()); err != nil {
			if isNotFoundError(err) {
				return nil
			}
			return resource.NonRetryableError(fmt.Errorf("do %s failed in delete db param group %s, %s", "DescribeUDBInstance", d.Id(), err))
		}

		return resource.RetryableError(fmt.Errorf("delete db param group but it still exists"))
	})
}

func resourceUCloudDBParameterHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-", m["key"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["value"].(string)))

	return hashcode.String(buf.String())
}
