cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.6.0"
  sha256 arm:   "dd07ef8bd9693d8a0341973129bbaabf27cff0b6a8759b38d1019fda04363647",
         intel: "e0bee62cdfb7c35609ac452e0d5a1f3dd8816394e0fb3e01c3c88a8a46ad7489"

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
