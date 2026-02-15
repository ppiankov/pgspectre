# Homebrew formula for pgspectre
# To use: brew install ppiankov/tap/pgspectre
# Or: brew tap ppiankov/tap && brew install pgspectre
#
# This formula is auto-updated by the release workflow.
# Manual edits will be overwritten on next release.

class Pgspectre < Formula
  desc "PostgreSQL schema and usage auditor — detects drift between code and database"
  homepage "https://github.com/ppiankov/pgspectre"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/ppiankov/pgspectre/releases/download/VERSION/pgspectre_VNUM_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    end
    on_intel do
      url "https://github.com/ppiankov/pgspectre/releases/download/VERSION/pgspectre_VNUM_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/ppiankov/pgspectre/releases/download/VERSION/pgspectre_VNUM_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    end
    on_intel do
      url "https://github.com/ppiankov/pgspectre/releases/download/VERSION/pgspectre_VNUM_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  def install
    bin.install "pgspectre"
  end

  test do
    assert_match "pgspectre", shell_output("#{bin}/pgspectre version")
  end
end
