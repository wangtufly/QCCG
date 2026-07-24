cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.7.0"
  sha256 arm:   "b0fc5fb297618c0b31f400069286dc2b113b862feedd5cfce8ffb49fece6c188",
         intel: "22a080751e21894cadbc04f5474de58dff38c646e2b30f5574826e9d3c90fc9c"

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
