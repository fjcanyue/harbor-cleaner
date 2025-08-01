# Harbor Cleaner

![Go Version](https://img.shields.io/badge/Go-1.18%2B-blue.svg)
![License](https://img.shields.io/badge/License-MIT-green.svg)

一个灵活、可配置且注重安全的 Go 脚本，用于清理 Harbor 镜像仓库中的过期镜像。该脚本可以通过命令行标志进行精细控制，并提供详细的日志和执行摘要，非常适合集成到 CI/CD 或定时任务 (CronJob) 中。

## ✨ 功能亮点

- **智能保留策略**: 保留每个仓库最新的 N 个版本，并可限制其中 `SNAPSHOT` 版本的最大数量。
- **安全第一**: 默认启用 **Dry Run (演练)** 模式，在不执行任何删除操作的情况下预览将要执行的清理任务。
- **项目白名单**: 可指定一个或多个项目进行扫描，避免对生产或关键项目造成意外影响。
- **全面的日志记录**: 所有操作都会实时输出到控制台，并同时保存到带时间戳的日志文件 (`harbor-cleaner-YYYYMMDD-HHMMSS.log`) 中，便于审计和排错。
- **清晰的汇总报告**: 脚本执行结束后，会提供一份详细的摘要，包括扫描的项目/仓库数量和已删除的镜像总数。
- **高度可配置**: 所有参数均通过命令行标志传递，易于在不同环境中使用。

## ⚙️ 先决条件

1.  **Go 环境**: 需要安装 Go 1.18 或更高版本。
2.  **Harbor 访问权限**: 需要能够访问您的 Harbor 实例的 API。
3.  **Harbor 机器人账户 (强烈推荐)**: 为了安全，建议在 Harbor 中创建一个专用的[机器人账户](https://goharbor.io/docs/2.10.0/user-guide/robot-accounts/)，并授予其执行清理任务所需的最小权限：
    -   **Project**: `list` (仓库列表)
    -   **Repository**: `read` (仓库读取)
    -   **Artifact**: `read`, `delete` (镜像读取与删除)

## 🚀 安装与编译

1.  **克隆仓库**
    ```bash
    git clone https://github.com/fjcanyue/harbor-cleaner
    cd harbor-cleaner
    ```

2.  **编译脚本**
    ```bash
    go build -o harbor-cleaner main.go
    ```
    这将在当前目录下生成一个名为 `harbor-cleaner` 的可执行文件。

## 📖 使用方法

### 命令行参数

| 参数                 | 默认值                                 | 描述                                                                          |
| -------------------- | -------------------------------------- | ----------------------------------------------------------------------------- |
| `-harbor.url`        | `""`                                   | **(必需)** Harbor API 的 URL (例如: `https://harbor.example.com`)               |
| `-harbor.user`       | `""`                                   | **(必需)** Harbor 用户名或机器人账户名称 (例如: `robot$mycleaner`)            |
| `-harbor.password`   | `""`                                   | **(必需)** Harbor 密码或机器人账户的 Token                                    |
| `-keep.last`         | `10`                                   | 每个仓库中需要保留的最新镜像数量。                                            |
| `-keep.snapshots`    | `2`                                    | 在保留的最新镜像中，最多允许存在的 `SNAPSHOT` 镜像数量。                      |
| `-dry-run`           | `true`                                 | 若为 `true`，则只打印将要执行的操作，不实际删除。**要执行删除，请设置为 `false`**。 |
| `-project.whitelist` | `""`                                   | 仅扫描指定的项目，项目名之间用逗号分隔。如果为空，则扫描所有项目。            |
| `-page-size`         | `100`                                  | 每次 API 请求获取的项目数量（用于分页）。                                     |

### 示例工作流

我们强烈建议您遵循以下步骤来安全地使用此脚本。

#### 第 1 步: 使用 Dry Run (演练) 模式进行预览

首次运行时，始终使用 `dry-run` 模式并配合 `-project.whitelist` 来限制范围，以验证清理策略是否符合预期。

```bash
./harbor-cleaner \
  -harbor.url="https://harbor.mycompany.com" \
  -harbor.user="robot$mycleaner" \
  -harbor.password="your-robot-token-here" \
  -project.whitelist="dev-team,staging-apps" \
  -keep.last=15 \
  -keep.snapshots=3
```

#### 第 2 步: 检查日志文件

脚本运行后，会生成一个日志文件，例如 `harbor-cleaner-20250801-015101.log`。仔细检查此文件，确认所有 `[DRY RUN] Would delete artifact...` 的条目都是您希望删除的镜像。

#### 第 3 步: 执行实际删除

确认 `dry-run` 的结果无误后，移除 `dry-run` 标志（或将其设置为 `false`）来执行真正的删除操作。

```bash
./harbor-cleaner \
  -harbor.url="https://harbor.mycompany.com" \
  -harbor.user="robot$mycleaner" \
  -harbor.password="your-robot-token-here" \
  -project.whitelist="dev-team,staging-apps" \
  -keep.last=15 \
  -keep.snapshots=3 \
  -dry-run=false
```

#### 第 4 步: 在 Harbor 中运行垃圾回收 (GC)

> ⚠️ **重要提示**: 此脚本删除的是镜像的标签（Tag/Artifact）。这并不会立即释放磁盘空间。您**必须**在 Harbor UI 中（`管理` -> `清理` -> `垃圾回收`）或通过 API 手动触发 **Garbage Collection (GC)** 来清理未被引用的镜像层 (blobs)，从而真正回收存储空间。

## 📝 许可证

本项目根据 [MIT 许可证](https://opensource.org/licenses/MIT) 发布。