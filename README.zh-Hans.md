# Contatto

[简体中文](README.zh-Hans.md)

Contatto 是透明镜像服务代理，主要用于：

1. 解决 Docker Hub、Quay.io、k8s.gcr.io 等站点访问速度慢的问题，提高镜像拉取速度。
2. 解决大规模镜像分发问题，降低对源站的压力。

# 安装

1. 下载 Contatto 压缩包，解压到对应的系统目录。
2. 配置 Contatto 配置文件，详见 [配置文件](#配置文件)。
3. 配置 Contatto 为系统服务，并启动。

## 配置文件

支持 json / yaml / toml 等 3 种格式的配置文件，默认使用 toml 格式。
