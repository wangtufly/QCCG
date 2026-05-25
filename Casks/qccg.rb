cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.5.4"
  sha256 arm:   "63527d8fbb2e5f7c2a78325667cd0cad5992333f6a0f4643c6e524c2ed6fd0e8",
         intel: "ebb6ae174902da69b12afe66a1684f598e6b5a26105023b58bfbe5304aac3a34"

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
