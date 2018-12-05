package ucloud

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/ucloud/ucloud-sdk-go/ucloud"
)

func resourceUCloudDBInstance() *schema.Resource {
	return &schema.Resource{
		Create: resourceUCloudDBInstanceCreate,
		Read:   resourceUCloudDBInstanceRead,
		Update: resourceUCloudDBInstanceUpdate,
		Delete: resourceUCloudDBInstanceDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"availability_zone": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"standby_zone": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"password": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				Sensitive:    true,
				ValidateFunc: validateInstancePassword,
			},

			"engine": &schema.Schema{
				Type:         schema.TypeString,
				ValidateFunc: validateStringInChoices([]string{"mysql", "percona", "postgresql"}),
				ForceNew:     true,
				Required:     true,
			},

			"engine_version": &schema.Schema{
				Type:         schema.TypeString,
				ValidateFunc: validateStringInChoices([]string{"5.5", "5.6", "5.7", "9.4", "9.6"}),
				ForceNew:     true,
				Required:     true,
			},

			"name": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateDBInstanceName,
			},

			"instance_storage": &schema.Schema{
				Type:         schema.TypeInt,
				Required:     true,
				ValidateFunc: validateDataDiskSize(20, 3000),
			},

			"parameter_group_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"instance_type": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateDBInstanceType,
			},

			"port": &schema.Schema{
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validateIntegerInRange(3306, 65535),
			},

			"instance_charge_type": &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "Month",
				ForceNew:     true,
				ValidateFunc: validateStringInChoices([]string{"Year", "Month", "Dynamic"}),
			},

			"instance_duration": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},

			"vpc_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"subnet_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"backup_count": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Default:  7,
				ForceNew: true,
			},

			"backup_begin_time": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},

			"backup_date": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"backup_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"backup_black_list": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validateDBInstanceBlackList,
				},
				Set: schema.HashString,
			},

			"tag": &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateInstanceName,
				Computed:     true,
			},

			"status": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"create_time": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"expire_time": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"modify_time": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceUCloudDBInstanceCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*UCloudClient)
	conn := client.udbconn

	engine := d.Get("engine").(string)
	// skip error because it has been validated by schema
	dbType, _ := parseDBInstanceType(d.Get("instance_type").(string))
	if dbType.Engine != engine {
		return fmt.Errorf("error in create db instance, engine of instance type %s must be same as engine %s", dbType.Engine, engine)
	}

	if dbType.Engine == "postgresql" && dbType.Type == "ha" {
		return fmt.Errorf("error in create db instance, high availability postgresql is not supported at this time")
	}

	if dbType.Engine == "mysql" && dbType.Type == "basic" {
		return fmt.Errorf("error in create db instance, basic mysql is no longer supported")
	}

	req := conn.NewCreateUDBInstanceRequest()
	req.Name = ucloud.String(d.Get("name").(string))
	req.AdminPassword = ucloud.String(d.Get("password").(string))
	zone := d.Get("availability_zone").(string)
	req.Zone = ucloud.String(zone)
	instanceStorage := d.Get("instance_storage").(int)
	req.DiskSpace = ucloud.Int(instanceStorage)
	memory := dbType.Memory * 1000
	if memory <= 8 && instanceStorage > 500 {
		return fmt.Errorf("the upper limit of instance storage is 500 when the memory is 8 or less")
	}

	if memory <= 24 && instanceStorage > 1000 {
		return fmt.Errorf("the upper limit of instance storage is 1000 when the memory between 12 and 24")
	}

	if memory == 32 && instanceStorage > 2000 {
		return fmt.Errorf("the upper limit of instance storage is 2000 when the memory is 32")
	}
	req.ChargeType = ucloud.String(d.Get("instance_charge_type").(string))
	req.Quantity = ucloud.Int(d.Get("instance_duration").(int))
	req.AdminUser = ucloud.String("root")
	req.InstanceType = ucloud.String("SATA_SSD")
	req.MemoryLimit = ucloud.Int(memory)
	req.InstanceMode = ucloud.String(dbMap.mustConvert(dbType.Type))
	engineVersion := d.Get("engine_version").(string)
	if engine == "mysql" || engine == "percona" {
		if err := checkStringIn(engineVersion, []string{"5.5", "5.6", "5.7"}); err != nil {
			return fmt.Errorf("The current engine version is not supported, %s", err)
		}
	} else {
		if err := checkStringIn(engineVersion, []string{"9.4", "9.6"}); err != nil {
			return fmt.Errorf("The current engine version is not supported, %s", err)
		}
	}
	req.DBTypeId = ucloud.String(strings.Join([]string{engine, engineVersion}, "-"))

	if val, ok := d.GetOk("tag"); ok {
		req.Tag = ucloud.String(val.(string))
	}

	if val, ok := d.GetOk("port"); ok {
		req.Port = ucloud.Int(val.(int))
	} else {
		if engine == "mysql" || engine == "percona" {
			req.Port = ucloud.Int(3306)
		}
		if engine == "postgresql" {
			req.Port = ucloud.Int(5432)
		}
	}

	if val, ok := d.GetOk("standby_zone"); ok && val.(string) != zone {
		req.BackupZone = ucloud.String(val.(string))
	}

	if val, ok := d.GetOk("backup_count"); ok {
		req.BackupCount = ucloud.Int(val.(int))
	}

	if val, ok := d.GetOk("backup_id"); ok {
		backupId, err := strconv.Atoi(val.(string))
		if err != nil {
			return err
		}
		req.BackupId = ucloud.Int(backupId)
	}

	if val, ok := d.GetOk("vpc_id"); ok {
		req.VPCId = ucloud.String(val.(string))
	}

	if val, ok := d.GetOk("subnet_id"); ok {
		req.SubnetId = ucloud.String(val.(string))
	}

	pgId, err := strconv.Atoi(d.Get("parameter_group_id").(string))
	if err != nil {
		return err
	}
	req.ParamGroupId = ucloud.Int(pgId)

	resp, err := conn.CreateUDBInstance(req)
	if err != nil {
		return fmt.Errorf("error in create db instance, %s", err)
	}

	d.SetId(resp.DBId)

	// after create db, we need to wait it initialized
	stateConf := client.dbWaitForState(d.Id(), []string{"Running"})

	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf("wait for db initialize failed in create db instance %s, %s", d.Id(), err)
	}

	return resourceUCloudDBInstanceUpdate(d, meta)
}

func resourceUCloudDBInstanceUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*UCloudClient)
	conn := client.udbconn

	d.Partial(true)

	if d.HasChange("name") && !d.IsNewResource() {
		req := conn.NewModifyUDBInstanceNameRequest()
		req.DBId = ucloud.String(d.Id())
		req.Name = ucloud.String(d.Get("name").(string))

		if _, err := conn.ModifyUDBInstanceName(req); err != nil {
			return fmt.Errorf("do %s failed in update db instance %s, %s", "ModifyUDBInstanceName", d.Id(), err)
		}
		d.SetPartial("name")
	}

	if d.HasChange("password") && !d.IsNewResource() {
		req := conn.NewModifyUDBInstancePasswordRequest()
		req.DBId = ucloud.String(d.Id())
		req.Password = ucloud.String(d.Get("password").(string))

		if _, err := conn.ModifyUDBInstancePassword(req); err != nil {
			return fmt.Errorf("do %s failed in update db instance %s, %s", "ModifyUDBInstancePassword", d.Id(), err)
		}
		d.SetPartial("password")
	}

	isSizeChanged := false
	sizeReq := conn.NewResizeUDBInstanceRequest()
	sizeReq.DBId = ucloud.String(d.Id())

	if d.HasChange("instance_type") && !d.IsNewResource() {
		engine := d.Get("engine").(string)
		old, new := d.GetChange("instance_type")

		oldType, _ := parseDBInstanceType(old.(string))

		newType, _ := parseDBInstanceType(new.(string))

		if newType.Engine != engine {
			return fmt.Errorf("error in update db instance, engine of instance type %s must be same as engine %s", newType.Engine, engine)
		}

		if newType.Type != oldType.Type {
			return fmt.Errorf("error in update db instance, db instance is not supported update the type of %q", "instance_type")
		}

		sizeReq.MemoryLimit = ucloud.Int(newType.Memory * 1000)
		isSizeChanged = true
	}

	if d.HasChange("instance_storage") && !d.IsNewResource() {
		sizeReq.DiskSpace = ucloud.Int(d.Get("instance_storage").(int))
		isSizeChanged = true
	}

	if isSizeChanged {
		db, err := client.describeDBInstanceById(d.Id())
		if err != nil {
			if isNotFoundError(err) {
				d.SetId("")
				return nil
			}
			return fmt.Errorf("do %s failed in update db instance %s, %s", "DescribeUDBInstance", d.Id(), err)
		}

		if db.InstanceMode == "Normal" {
			//update these attributes of db instance, we need to wait it stopped
			stopReq := conn.NewStopUDBInstanceRequest()
			stopReq.DBId = ucloud.String(d.Id())
			stopReq.Zone = ucloud.String(d.Get("availability_zone").(string))
			if db.State != "Shutoff" {
				_, err := conn.StopUDBInstance(stopReq)

				if err != nil {
					return fmt.Errorf("do %s failed in update db instance %s, %s", "StopUDBInstance", d.Id(), err)
				}

				// after stop db instance, we need to wait it stopped
				stateConf := client.dbWaitForState(d.Id(), []string{"Shutoff"})

				if _, err := stateConf.WaitForState(); err != nil {
					return fmt.Errorf("wait for stop db instance failed in update db instance %s, %s", d.Id(), err)
				}
			}

			if _, err := conn.ResizeUDBInstance(sizeReq); err != nil {
				return fmt.Errorf("do %s failed in update db instance %s, %s", "ResizeUDBInstance", d.Id(), err)
			}

			// after resize db instance, we need to wait it completed
			stateConf := client.dbWaitForState(d.Id(), []string{"Shutoff"})

			if _, err := stateConf.WaitForState(); err != nil {
				return fmt.Errorf("wait for resize db instance failed in update db instance %s, %s", d.Id(), err)
			}

			d.SetPartial("instance_storage")
			d.SetPartial("instance_type")

			if db.State == "Running" {
				// after update these attributes of db instance completed, we need start it
				startReq := conn.NewStartUDBInstanceRequest()
				startReq.DBId = ucloud.String(d.Id())
				startReq.Zone = ucloud.String(d.Get("availability_zone").(string))

				_, err = conn.StartUDBInstance(startReq)

				if err != nil {
					return fmt.Errorf("do %s failed in update db instance %s, %s", "StartUDBInstance", d.Id(), err)
				}

				//after start db instance, we need to wait it running
				stateConf = client.dbWaitForState(d.Id(), []string{"Running"})

				if _, err := stateConf.WaitForState(); err != nil {
					return fmt.Errorf("wait for start db instance failed in update db instance %s, %s", d.Id(), err)
				}
			}

		} else {
			if _, err := conn.ResizeUDBInstance(sizeReq); err != nil {
				return fmt.Errorf("do %s failed in update db instance %s, %s", "ResizeUDBInstance", d.Id(), err)
			}

			// after resize db instance, we need to wait it completed
			stateConf := client.dbWaitForState(d.Id(), []string{"Running", "Shutoff"})

			if _, err := stateConf.WaitForState(); err != nil {
				return fmt.Errorf("wait for resize db instance failed in update db instance %s, %s", d.Id(), err)
			}

			d.SetPartial("instance_storage")
			d.SetPartial("instance_type")
		}
	}

	//change parameter group id take effect until the db instance is restarted
	if d.HasChange("parameter_group_id") && !d.IsNewResource() {
		pgReq := client.pudbconn.NewChangeUDBParamGroupRequest()
		pgReq.DBId = ucloud.String(d.Id())
		pgReq.GroupId = ucloud.String(d.Get("parameter_group_id").(string))
		if _, err := client.pudbconn.ChangeUDBParamGroup(pgReq); err != nil {
			return fmt.Errorf("do %s failed in update db instance %s, %s", "ChangeUDBParamGroup", d.Id(), err)
		}

		resReq := conn.NewRestartUDBInstanceRequest()
		resReq.DBId = ucloud.String(d.Id())
		if _, err := conn.RestartUDBInstance(resReq); err != nil {
			return fmt.Errorf("do %s failed in update db instance %s, %s", "RestartUDBInstance", d.Id(), err)
		}

		// after change parameter group id , we need to wait it completed
		stateConf := client.dbWaitForState(d.Id(), []string{"Running", "Shutoff"})

		if _, err := stateConf.WaitForState(); err != nil {
			return fmt.Errorf("wait for change parameter group id failed in update db instance %s, %s", d.Id(), err)
		}
		d.SetPartial("parameter_group_id")
	}

	backupChanged := false
	buReq := conn.NewUpdateUDBInstanceBackupStrategyRequest()
	buReq.DBId = ucloud.String(d.Id())

	if d.HasChange("backup_date") {
		buReq.BackupDate = ucloud.String(d.Get("backup_date").(string))
		backupChanged = true
	}

	if d.HasChange("backup_begin_time") {
		buReq.BackupTime = ucloud.Int(d.Get("backup_begin_time").(int))
		backupChanged = true
	}

	if backupChanged {
		if _, err := conn.UpdateUDBInstanceBackupStrategy(buReq); err != nil {
			return fmt.Errorf("do %s failed in update db instance %s, %s", "UpdateUDBInstanceBackupStrategy", d.Id(), err)
		}

		d.SetPartial("backup_date")
		d.SetPartial("backup_begin_time")
	}

	if d.HasChange("backup_black_list") {
		blackList := strings.Join(ifaceToStringSlice(d.Get("backup_black_list").(*schema.Set).List()), ";")
		req := conn.NewEditUDBBackupBlacklistRequest()
		req.Blacklist = ucloud.String(blackList)
		req.DBId = ucloud.String(d.Id())

		if _, err := conn.EditUDBBackupBlacklist(req); err != nil {
			return fmt.Errorf("do %s failed in update db instance %s, %s", "EditUDBBackupBlacklist", d.Id(), err)
		}

		d.SetPartial("backup_black_list")
	}

	d.Partial(false)

	return resourceUCloudDBInstanceRead(d, meta)
}

func resourceUCloudDBInstanceRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*UCloudClient)

	db, err := client.describeDBInstanceById(d.Id())
	if err != nil {
		if isNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("do %s failed in read db instance %s, %s", "DescribeUDBInstance", d.Id(), err)
	}

	arr := strings.Split(db.DBTypeId, "-")
	d.Set("name", db.Name)
	d.Set("engine", arr[0])
	d.Set("engine_version", arr[1])
	d.Set("parameter_group_id", strconv.Itoa(db.ParamGroupId))
	d.Set("port", db.Port)
	d.Set("status", db.State)
	d.Set("instance_charge_type", db.ChargeType)
	d.Set("instance_storage", db.DiskSpace)
	d.Set("standby_zone", db.BackupZone)
	d.Set("availability_zone", db.Zone)
	d.Set("backup_count", db.BackupCount)
	d.Set("backup_begin_time", db.BackupBeginTime)
	d.Set("backup_date", db.BackupDate)

	backupBlackList := strings.Split(db.BackupBlacklist, ";")
	d.Set("backup_black_list", backupBlackList)
	d.Set("tag", db.Tag)
	d.Set("create_time", timestampToString(db.CreateTime))
	d.Set("expire_time", timestampToString(db.ExpiredTime))
	d.Set("modify_time", timestampToString(db.ModifyTime))
	var dbType dbInstanceType
	dbType.Memory = db.MemoryLimit / 1000
	dbType.Engine = arr[0]
	dbType.Type = dbMap.mustUnconvert(db.InstanceMode)
	d.Set("instance_type", fmt.Sprintf("%s-%s-%d", dbType.Engine, dbType.Type, dbType.Memory))

	return nil
}

func resourceUCloudDBInstanceDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*UCloudClient)
	conn := client.udbconn

	req := conn.NewDeleteUDBInstanceRequest()
	req.DBId = ucloud.String(d.Id())
	stopReq := conn.NewStopUDBInstanceRequest()
	stopReq.DBId = ucloud.String(d.Id())

	return resource.Retry(5*time.Minute, func() *resource.RetryError {
		db, err := client.describeDBInstanceById(d.Id())
		if err != nil {
			if isNotFoundError(err) {
				return nil
			}
			return resource.NonRetryableError(err)
		}

		if db.State != "Shutoff" {
			if _, err := conn.StopUDBInstance(stopReq); err != nil {
				return resource.RetryableError(fmt.Errorf("do %s failed in delete db instance %s, %s", "StopUDBInstance", d.Id(), err))
			}

			// after instance stop, we need to wait it stoped
			stateConf := client.dbWaitForState(d.Id(), []string{"Shutoff"})

			if _, err := stateConf.WaitForState(); err != nil {
				return resource.RetryableError(fmt.Errorf("wait for db instance stop failed in delete db instance %s, %s", d.Id(), err))
			}
		}

		if _, err := conn.DeleteUDBInstance(req); err != nil {
			return resource.NonRetryableError(fmt.Errorf("error in delete db instance %s, %s", d.Id(), err))
		}

		if _, err := client.describeDBInstanceById(d.Id()); err != nil {
			if isNotFoundError(err) {
				return nil
			}
			return resource.NonRetryableError(fmt.Errorf("do %s failed in delete db instance %s, %s", "DescribeUDBInstance", d.Id(), err))
		}

		return resource.RetryableError(fmt.Errorf("delete db instance but it still exists"))
	})
}
