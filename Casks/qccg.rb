cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.6.6"
  sha256 arm:   "13fa0acb90b24347981c5e9a9b1fb5a230b828238e602a7fa628867ef420b98f",
         intel: "3ea3791bde12955598bf9e4d41dc018332f0ff6b642170b56b34ceb5bc672b31"

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
