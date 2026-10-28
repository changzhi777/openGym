# android/ — 沐禾健身 Android 壳（Capacitor 7）

> 本目录是 Android 原生工程（Gradle 8.11.1 / compileSdk 35 / minSdk 23），由 Capacitor 管理。
> **Web 源码在 `../src/`** —— 本目录只放壳与原生插件，业务代码不要写在这里。

## 用 Android Studio 打开（导入）

1. **Open** → 选择本目录（`frontend/android`，注意不是 `frontend/`）
2. 首次打开等 Gradle Sync 跑完（wrapper 会自动下载 Gradle 8.11.1）
3. **Gradle JDK 必须是 21**：`Settings → Build, Execution, Deployment → Build Tools → Gradle → Gradle JDK`
   选 21（本机已装 JetBrains JBR 21；命令行对应 `/usr/libexec/java_home -v 21`）
4. `local.properties`（sdk.dir）由 IDE 自动生成，勿提交 —— 已在 .gitignore

## 改动后的标准循环（三个坑，按顺序）

```bash
cd frontend
npm run build          # ① web 构建（dist/）
npx cap sync android   # ② 同步 dist + 插件到 android 工程
# 然后回 Android Studio Run，或命令行：
cd android && JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew assembleDebug
```

- ⚠️ **坑 1**：`npm run build:mobile` 是出 APK 的完整链（含 stage-media 媒体资产 6554×2），但它最后会同步 iOS 并**因 CocoaPods 未装而报错** —— Android 开发日常用上面的 `cap sync android` 分步版即可。
- ⚠️ **坑 2**：**发布 APK 必须 `build:mobile` 链**（含 `stage-media`），只 `npm run build` 的包动画全黑。装 CocoaPods（`brew install cocoapods`）后 build:mobile 可完整跑通。
- ⚠️ **坑 3**：JDK 版本 <21 会在 Gradle 编译期直接失败；Node 用 22 口径（与仓库测试门一致）。

## 命令速查

```bash
# Debug APK → app/build/outputs/apk/debug/app-debug.apk
JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew assembleDebug

# Release（签名配置见 app/build.gradle）
JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew assembleRelease

# 单测（少量 JUnit：RestTone 等）
JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew testDebugUnitTest
```

## 与仓库的关系

- 原生改动（Java 插件/Manifest/图标）直接提交本目录；`cap sync` 会保留手改
- 生成物勿提交：`app/src/main/assets/public/`、`capacitor-cordova-android-plugins/`、`local.properties`、`app/build/`（.gitignore 已覆盖）
- 移动端文档：[../docs/MOBILE.md](../docs/MOBILE.md) · fork 构建差异：[../FORK.md](../FORK.md)
