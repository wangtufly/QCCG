cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.5.4"
  sha256 arm:   "9f6761c092b9f9ea7b01835f184f77fea1c95e2000f890e68fb431f08421fe8b",
         intel: "a839d05e43a5bf040099b8150bd4e648a99d166b63cb10bace0d1d2c14c64086"

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
