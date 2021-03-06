package main

import (
	"fmt"
	"github.com/gobs/args"
	"github.com/iikira/BaiduPCS-Go/pcscache"
	"github.com/iikira/BaiduPCS-Go/pcscommand"
	"github.com/iikira/BaiduPCS-Go/pcsconfig"
	"github.com/iikira/BaiduPCS-Go/pcsliner"
	"github.com/iikira/BaiduPCS-Go/pcsutil"
	"github.com/iikira/BaiduPCS-Go/pcsverbose"
	"github.com/iikira/BaiduPCS-Go/pcsweb"
	"github.com/urfave/cli"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

var (
	// Version 版本号
	Version = "v3.3.Beta1"

	historyFilePath = pcsutil.ExecutablePathJoin("pcs_command_history.txt")
	reloadFn        = func(c *cli.Context) error {
		pcscommand.ReloadIfInConsole()
		return nil
	}
)

func init() {
	pcsconfig.Init()
	pcscommand.ReloadInfo()

	pcscache.DirCache.GC() // 启动缓存垃圾回收
}

// getSubArgs 获取子命令参数
func getSubArgs(c *cli.Context) (sargs []string) {
	for i := 0; c.Args().Get(i) != ""; i++ {
		sargs = append(sargs, c.Args().Get(i))
	}
	return
}

func main() {
	app := cli.NewApp()
	app.Name = "BaiduPCS-Go"
	app.Version = Version
	app.Author = "iikira/BaiduPCS-Go: https://github.com/iikira/BaiduPCS-Go"
	app.Usage = "百度网盘工具箱 for " + runtime.GOOS + "/" + runtime.GOARCH
	app.Description = `BaiduPCS-Go 使用 Go语言编写, 为操作百度网盘, 提供实用功能.
	具体功能, 参见 COMMANDS 列表

	特色:
		网盘内列出文件和目录, 支持通配符匹配路径;
		下载网盘内文件, 支持网盘内目录 (文件夹) 下载, 支持多个文件或目录下载, 支持断点续传和高并发高速下载.
	
	---------------------------------------------------
	前往 https://github.com/iikira/BaiduPCS-Go 以获取更多帮助信息!
	前往 https://github.com/iikira/BaiduPCS-Go/releases 以获取程序更新信息!
	---------------------------------------------------`

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:        "verbose",
			Usage:       "启用调试",
			EnvVar:      "BAIDUPCS_GO_VERBOSE",
			Destination: &pcsverbose.IsVerbose,
		},
	}
	app.Action = func(c *cli.Context) {
		if c.NArg() != 0 {
			fmt.Printf("未找到命令: %s\n运行命令 %s help 获取帮助\n", c.Args().Get(0), app.Name)
			return
		}
		cli.ShowAppHelp(c)
		pcsverbose.Verbosef("这是一条调试信息\n\n")

		line := pcsliner.NewLiner()

		var err error
		line.History, err = pcsliner.NewLineHistory(historyFilePath)
		if err != nil {
			fmt.Printf("警告: 读取历史命令文件错误, %s\n", err)
		}

		line.ReadHistory()
		defer func() {
			line.DoWriteHistory()
			line.Close()
		}()

		// tab 自动补全命令
		line.State.SetCompleter(func(line string) (s []string) {
			cmds := cli.CommandsByName(app.Commands)

			for k := range cmds {
				if !strings.HasPrefix(cmds[k].FullName(), line) {
					continue
				}
				s = append(s, cmds[k].FullName()+" ")
			}
			return s
		})

		for {
			var (
				prompt          string
				activeBaiduUser = pcsconfig.Config.MustGetActive()
			)

			if activeBaiduUser.Name != "" {
				// 格式: BaiduPCS-Go:<工作目录> <百度ID>$
				// 工作目录太长的话会自动缩略
				prompt = app.Name + ":" + pcsutil.ShortDisplay(path.Base(activeBaiduUser.Workdir), 20) + " " + activeBaiduUser.Name + "$ "
			} else {
				// BaiduPCS-Go >
				prompt = app.Name + " > "
			}

			commandLine, err := line.State.Prompt(prompt)
			if err != nil {
				fmt.Println(err)
				return
			}

			line.State.AppendHistory(commandLine)

			cmdArgs := args.GetArgs(commandLine)
			if len(cmdArgs) == 0 {
				continue
			}

			s := []string{os.Args[0]}
			s = append(s, cmdArgs...)

			// 恢复原始终端状态
			// 防止运行命令时程序被结束, 终端出现异常
			line.Pause()

			c.App.Run(s)

			line.Resume()
		}
	}

	app.Commands = []cli.Command{
		{
			Name:     "web",
			Usage:    "启用 web 客户端 (测试中)",
			Category: "其他",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				fmt.Printf("web 客户端功能为实验性功能, 测试中, 打开 http://localhost:%d 查看效果\n", c.Uint("port"))
				fmt.Println(pcsweb.StartServer(c.Uint("port")))
				return nil
			},
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "port",
					Usage: "自定义端口",
					Value: 8080,
				},
			},
		},
		{
			Name:     "run",
			Usage:    "执行系统命令",
			Category: "其他",
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				cmd := exec.Command(c.Args().First(), c.Args().Tail()...)
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr

				err := cmd.Run()
				if err != nil {
					fmt.Println(err)
				}

				return nil
			},
		},
		{
			Name:  "login",
			Usage: "登录百度账号",
			Description: `
	示例:
		BaiduPCS-Go login
		BaiduPCS-Go login --username=liuhua
		BaiduPCS-Go login --bduss=123456789

	常规登录:
		按提示一步一步来即可.

	百度BDUSS获取方法:
		参考这篇 Wiki: https://github.com/iikira/BaiduPCS-Go/wiki/关于-获取百度-BDUSS
		或者百度搜索: 获取百度BDUSS
`,
			Category: "百度帐号",
			Before:   reloadFn,
			After:    reloadFn,
			Action: func(c *cli.Context) error {
				var bduss, ptoken, stoken string
				if c.IsSet("bduss") {
					bduss = c.String("bduss")
				} else if c.NArg() == 0 {
					var err error
					bduss, ptoken, stoken, err = pcscommand.RunLogin(c.String("username"), c.String("password"))
					if err != nil {
						fmt.Println(err)
						return err
					}
				} else {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				username, err := pcsconfig.Config.SetBDUSS(bduss, ptoken, stoken)
				if err != nil {
					fmt.Println(err)
					return nil
				}

				fmt.Println("百度帐号登录成功:", username)
				return nil
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "username",
					Usage: "登录百度帐号的用户名(手机号/邮箱/用户名)",
				},
				cli.StringFlag{
					Name:  "password",
					Usage: "登录百度帐号的用户名的密码",
				},
				cli.StringFlag{
					Name:  "bduss",
					Usage: "使用百度 BDUSS 来登录百度帐号",
				},
			},
		},
		{
			Name:    "su",
			Aliases: []string{"chuser"}, // 兼容旧版本
			Usage:   "切换已登录的百度帐号",
			Description: fmt.Sprintf("%s\n   示例:\n\n      %s\n      %s\n",
				"如果运行该条命令没有提供参数, 程序将会列出所有的百度帐号, 供选择切换",
				filepath.Base(os.Args[0])+" su --uid=123456789",
				filepath.Base(os.Args[0])+" su",
			),
			Category: "百度帐号",
			Before:   reloadFn,
			After:    reloadFn,
			Action: func(c *cli.Context) error {
				if len(pcsconfig.Config.BaiduUserList) == 0 {
					fmt.Println("未设置任何百度帐号, 不能切换")
					return nil
				}

				var uid uint64
				if c.IsSet("uid") {
					if pcsconfig.Config.CheckUIDExist(c.Uint64("uid")) {
						uid = c.Uint64("uid")
					} else {
						fmt.Println("切换用户失败, uid 不存在")
					}
				} else if c.NArg() == 0 {
					cli.HandleAction(app.Command("loglist").Action, c)

					// 提示输入 index
					var index string
					fmt.Printf("输入要切换帐号的 index 值 > ")
					_, err := fmt.Scanln(&index)
					if err != nil {
						return nil
					}

					if n, err := strconv.Atoi(index); err == nil && n >= 0 && n < len(pcsconfig.Config.BaiduUserList) {
						uid = pcsconfig.Config.BaiduUserList[n].UID
					} else {
						fmt.Println("切换用户失败, 请检查 index 值是否正确")
					}
				} else {
					cli.ShowCommandHelp(c, c.Command.Name)
				}

				if uid == 0 {
					return nil
				}

				pcsconfig.Config.BaiduActiveUID = uid
				if err := pcsconfig.Config.Save(); err != nil {
					fmt.Println(err)
					return nil
				}

				fmt.Printf("切换用户成功, %v\n", pcsconfig.Config.MustGetActive().Name)
				return nil

			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "uid",
					Usage: "百度帐号 uid 值",
				},
			},
		},
		{
			Name:     "logout",
			Usage:    "退出当前登录的百度帐号",
			Category: "百度帐号",
			Before:   reloadFn,
			After:    reloadFn,
			Action: func(c *cli.Context) error {
				if len(pcsconfig.Config.BaiduUserList) == 0 {
					fmt.Println("未设置任何百度帐号, 不能退出")
					return nil
				}

				var (
					au      = pcsconfig.Config.MustGetActive()
					confirm string
				)

				fmt.Printf("确认退出百度帐号: %s ? (y/n) > ", au.Name)
				_, err := fmt.Scanln(&confirm)
				if err != nil || (confirm != "y" && confirm != "Y") {
					return err
				}

				err = pcsconfig.Config.DeleteBaiduUserByUID(au.UID)
				if err != nil {
					fmt.Printf("退出用户 %s, 失败, 错误: %s\n", au.Name, err)
				}

				fmt.Printf("退出用户成功, %s\n", au.Name)
				return nil
			},
		},
		{
			Name:     "loglist",
			Usage:    "获取当前帐号, 和所有已登录的百度帐号",
			Category: "百度帐号",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				au := pcsconfig.Config.MustGetActive()

				fmt.Printf("\n当前帐号 uid: %d, 用户名: %s\n", au.UID, au.Name)

				fmt.Println(pcsconfig.Config.BaiduUserList.String())

				return nil
			},
		},
		{
			Name:     "quota",
			Usage:    "获取配额, 即获取网盘总空间, 和已使用空间",
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				pcscommand.RunGetQuota()
				return nil
			},
		},
		{
			Name:      "cd",
			Category:  "百度网盘",
			Usage:     "切换工作目录",
			UsageText: fmt.Sprintf("%s cd <目录 绝对路径或相对路径>", filepath.Base(os.Args[0])),
			Before:    reloadFn,
			After:     reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				pcscommand.RunChangeDirectory(c.Args().Get(0), c.Bool("l"))

				return nil
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "l",
					Usage: "切换工作目录后自动列出工作目录下的文件和目录",
				},
			},
		},
		{
			Name:      "ls",
			Aliases:   []string{"l", "ll"},
			Usage:     "列出当前工作目录内的文件和目录 或 指定目录内的文件和目录",
			UsageText: fmt.Sprintf("%s ls <目录 绝对路径或相对路径>", filepath.Base(os.Args[0])),
			Category:  "百度网盘",
			Before:    reloadFn,
			Action: func(c *cli.Context) error {
				pcscommand.RunLs(c.Args().Get(0))
				return nil
			},
		},
		{
			Name:      "pwd",
			Usage:     "输出当前所在目录 (工作目录)",
			UsageText: fmt.Sprintf("%s pwd", filepath.Base(os.Args[0])),
			Category:  "百度网盘",
			Before:    reloadFn,
			Action: func(c *cli.Context) error {
				fmt.Println(pcsconfig.Config.MustGetActive().Workdir)
				return nil
			},
		},
		{
			Name:      "meta",
			Usage:     "获取单个文件/目录的元信息 (详细信息)",
			UsageText: fmt.Sprintf("%s meta <文件/目录 绝对路径或相对路径>", filepath.Base(os.Args[0])),
			Category:  "百度网盘",
			Before:    reloadFn,
			Action: func(c *cli.Context) error {
				pcscommand.RunGetMeta(c.Args().Get(0))
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "删除 单个/多个 文件/目录",
			UsageText: fmt.Sprintf("%s rm <网盘文件或目录的路径1> <文件或目录2> <文件或目录3> ...", filepath.Base(os.Args[0])),
			Description: fmt.Sprintf("\n   %s\n   %s\n",
				"注意: 删除多个文件和目录时, 请确保每一个文件和目录都存在, 否则删除操作会失败.",
				"被删除的文件或目录可在文件回收站找回.",
			),
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				pcscommand.RunRemove(getSubArgs(c)...)
				return nil
			},
		},
		{
			Name:      "mkdir",
			Usage:     "创建目录",
			UsageText: fmt.Sprintf("%s mkdir <目录 绝对路径或相对路径> ...", filepath.Base(os.Args[0])),
			Category:  "百度网盘",
			Before:    reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				pcscommand.RunMkdir(c.Args().Get(0))
				return nil
			},
		},
		{
			Name:  "cp",
			Usage: "拷贝(复制) 文件/目录",
			UsageText: fmt.Sprintf(
				"%s cp <文件/目录> <目标 文件/目录>\n   %s cp <文件/目录1> <文件/目录2> <文件/目录3> ... <目标目录>",
				filepath.Base(os.Args[0]),
				filepath.Base(os.Args[0]),
			),
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() <= 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				pcscommand.RunCopy(getSubArgs(c)...)
				return nil
			},
		},
		{
			Name:  "mv",
			Usage: "移动/重命名 文件/目录",
			UsageText: fmt.Sprintf(
				"移动\t: %s mv <文件/目录1> <文件/目录2> <文件/目录3> ... <目标目录>\n   重命名: %s mv <文件/目录> <重命名的文件/目录>",
				filepath.Base(os.Args[0]),
				filepath.Base(os.Args[0]),
			),
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() <= 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				pcscommand.RunMove(getSubArgs(c)...)
				return nil
			},
		},
		{
			Name:        "download",
			Aliases:     []string{"d"},
			Usage:       "下载文件或目录",
			UsageText:   fmt.Sprintf("%s download <网盘文件或目录的路径1> <文件或目录2> <文件或目录3> ...", filepath.Base(os.Args[0])),
			Description: "下载的文件将会保存到, 程序所在目录的 download/ 目录 (文件夹).\n   已支持目录下载.\n   已支持多个文件或目录下载.\n   自动跳过下载重名的文件! \n",
			Category:    "百度网盘",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				pcscommand.RunDownload(c.Bool("test"), c.Int("p"), getSubArgs(c))
				return nil
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "test",
					Usage: "测试下载, 此操作不会保存文件到本地",
				},
				cli.IntFlag{
					Name:  "p",
					Usage: "指定下载线程数",
				},
			},
		},
		{
			Name:        "upload",
			Aliases:     []string{"u"},
			Usage:       "上传文件或目录",
			UsageText:   fmt.Sprintf("%s upload <本地文件或目录的路径1> <文件或目录2> <文件或目录3> ... <网盘的目标目录>", filepath.Base(os.Args[0])),
			Description: "上传的文件将会保存到 网盘的目标目录.\n   遇到同名文件将会自动覆盖! \n",
			Category:    "百度网盘",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() <= 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				subArgs := getSubArgs(c)

				pcscommand.RunUpload(subArgs[:c.NArg()-1], subArgs[c.NArg()-1])
				return nil
			},
		},
		{
			Name:        "rapidupload",
			Aliases:     []string{"ru"},
			Usage:       "手动秒传文件",
			UsageText:   fmt.Sprintf("%s rapidupload -length=<文件的大小> -md5=<文件的md5值> -slicemd5=<文件前256KB切片的md5值> -crc32=<文件的crc32值(可选)> <保存的网盘路径, 需包含文件名>", filepath.Base(os.Args[0])),
			Description: "上传的文件将会保存到 网盘的目标目录.\n   遇到同名文件将会自动覆盖! \n",
			Category:    "百度网盘",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() <= 0 || !c.IsSet("md5") || !c.IsSet("slicemd5") || !c.IsSet("length") {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				pcscommand.RunRapidUpload(c.Args().Get(0), c.String("md5"), c.String("slicemd5"), c.String("crc32"), c.Int64("length"))
				return nil
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "md5",
					Usage: "文件的 md5 值",
				},
				cli.StringFlag{
					Name:  "slicemd5",
					Usage: "文件前 256KB 切片的 md5 值",
				},
				cli.StringFlag{
					Name:  "crc32",
					Usage: "文件的 crc32 值 (可选)",
				},
				cli.Int64Flag{
					Name:  "length",
					Usage: "文件的大小",
				},
			},
		},
		{
			Name:        "sumfile",
			Aliases:     []string{"sf"},
			Usage:       "获取文件的秒传信息",
			UsageText:   fmt.Sprintf("%s sumfile <本地文件的路径>", filepath.Base(os.Args[0])),
			Description: "获取文件的大小, md5, 前256KB切片的md5, crc32, 可用于秒传文件.",
			Category:    "其他",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() <= 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				lp, err := pcscommand.GetFileSum(c.Args().Get(0), false)
				if err != nil {
					fmt.Println(err)
					return err
				}

				fmt.Printf(
					"\n[%s]:\n文件大小: %d, md5: %x, 前256KB切片的md5: %x, crc32: %d, \n\n秒传命令: %s rapidupload -length=%d -md5=%x -slicemd5=%x -crc32=%d %s\n\n",
					c.Args().Get(0),
					lp.Length, lp.MD5, lp.SliceMD5, lp.CRC32,
					os.Args[0],
					lp.Length, lp.MD5, lp.SliceMD5, lp.CRC32,
					filepath.Base(c.Args().Get(0)),
				)

				return nil
			},
		},
		{
			Name:      "set",
			Usage:     "修改程序配置项",
			UsageText: fmt.Sprintf("%s set OptionName Value", filepath.Base(os.Args[0])),
			Description: `
可设置的值:
	OptionName		Value
	------------------------------------------------------
	appid	百度 PCS 应用ID

	user_agent	浏览器标识
	cache_size	下载缓存, 如果硬盘占用高或下载速度慢, 请尝试调大此值, 建议值 ( 1024 ~ 16384 )
	max_parallel	下载最大并发量 - 建议值 ( 50 ~ 500 )
	savedir	下载文件的储存目录

例子:
	set appid 260149
	set cache_size 2048
	set max_parallel 250
	set savedir D:/download
`,
			Category: "配置",
			Before:   reloadFn,
			After:    reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() < 2 || c.Args().Get(1) == "" {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				err := pcsconfig.Config.SetConfig(c.Args().Get(0), c.Args().Get(1)) // 设置
				if err != nil {
					fmt.Println(err)
					cli.ShowCommandHelp(c, "set")
				}
				return nil
			},
		},
		{
			Name:    "quit",
			Aliases: []string{"exit"},
			Usage:   "退出程序",
			Action: func(c *cli.Context) error {
				return cli.NewExitError("", 0)
			},
			Hidden:   true,
			HideHelp: true,
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Run(os.Args)
}

// �
