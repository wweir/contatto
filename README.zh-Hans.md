# Contatto

[English](README.md)

Contatto 是透明镜像服务代理，主要用于：

1. 解决 Docker Hub、Quay.io、k8s.gcr.io 等站点访问速度慢的问题，提高镜像拉取速度。
2. 解决大规模镜像分发问题，降低对源站的压力。

## 安装

### 使用发布包

从[发布页面](https://github.com/wweir/contatto/releases)下载适合您系统的软件包：

#### Debian/Ubuntu (.deb)
```bash
wget https://github.com/wweir/contatto/releases/download/v<版本>/contatto_<版本>_amd64.deb
sudo dpkg -i contatto_<版本>_amd64.deb
sudo apt-get install -f  # 如需要修复依赖关系
```

#### Arch Linux (.pkg.tar.xz)
```bash
wget https://github.com/wweir/contatto/releases/download/v<版本>/contatto-<版本>-x86_64.pkg.tar.xz
sudo pacman -U contatto-<版本>-x86_64.pkg.tar.xz
```

#### CentOS/RHEL/Fedora (.rpm)
```bash
wget https://github.com/wweir/contatto/releases/download/v<版本>/contatto-<版本>-x86_64.rpm
sudo rpm -ivh contatto-<版本>-x86_64.rpm
# 或者
sudo dnf install contatto-<版本>-x86_64.rpm
```

#### 通用 Linux (.tar.gz)
```bash
wget https://github.com/wweir/contatto/releases/download/v<版本>/contatto-linux-amd64.tar.gz
tar -xzf contatto-linux-amd64.tar.gz
sudo ./install.sh  # 或手动复制文件
```

### 从源码安装

1. 克隆仓库
2. 构建二进制文件：`make build`
3. 将二进制文件复制到目标位置
4. 配置服务（参见[配置文件](#配置文件)）

## 配置文件

Contatto 支持 JSON、YAML 和 TOML 等 3 种格式的配置文件，默认使用 TOML 格式。

示例配置文件（`/etc/contatto.toml`）：

```toml
# 完整配置选项请参见 config/contatto.example.toml
```

## 使用方法

### 作为系统服务

安装后，启用并启动服务：

```bash
# 启用服务
sudo systemctl enable contatto@contatto.service

# 启动服务
sudo systemctl start contatto@contatto.service

# 检查状态
sudo systemctl status contatto@contatto.service
```

## 开发

### 构建

```bash
# 构建二进制文件
make build

# 运行测试
make test

# 构建软件包
make deb-simple     # Debian 包
make rpm-fallback   # RPM 包
make arch-fallback  # Arch 包
```

### GitHub Actions

本项目使用 GitHub Actions 进行自动化测试和软件包构建。CI/CD 流水线包括：

1. **master.yaml**：在每次推送到 main/master 分支时运行测试
2. **release.yaml**：在标签推送时为多个架构构建软件包

最近的修复包括将 Arch Linux 软件包构建路径从 `deploy/arch/` 纠正为 `deploy/archlinux/`。

## 贡献

1. Fork 仓库
2. 创建功能分支
3. 进行更改
4. 如适用，添加测试
5. 提交 Pull Request

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。
