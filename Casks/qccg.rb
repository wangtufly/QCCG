cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.7.1"
  sha256 arm:   "14244e3896ee99d40aa6b32364f6b1efd694e1b035c4aab77e5c85019edc3745",
         intel: "c294f3aa25a8d7ad289dc1e01721da396bdcf4b9a4e9b20a3a57afb21e6ef88b"

  url "https://github.com/wangtufly/QCCG/releases/download/v#{version}/QCCG-v#{version}-darwin-#{arch}.dmg"
  name "QCCG"
  desc "Qoder Claude Codex Gemini Gateway"
  homepage "https://github.com/wangtufly/QCCG"

  depends_on macos: ">= :monterey"

  app "qccg.app", target: "QCCG.app"

  zap trash: [
    "~/Library/Application Support/QCCG",
    "~/Library/Preferences/com.qccg.app.plist",
  ]
end
