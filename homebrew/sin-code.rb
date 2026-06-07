class SinCode < Formula
  desc "SIN-Code unified toolchain — 19 subcommands in one binary (13 MCP + 6 CLI-only)"
  homepage "https://github.com/OpenSIN-Code/SIN-Code-Bundle"
  version "1.0.7"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.7/sin-code-darwin-arm64.tar.gz"
      sha256 "12dde8f8d92121511985509a1ef3c9058cd46155ea5e062b6aa3709b7765451a"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.7/sin-code-darwin-amd64.tar.gz"
      sha256 "72fb74600aab5e8b94fcef3edd4b88070edc81bad0d0bdc236e3b9cb5ab83b89"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.7/sin-code-linux-arm64.tar.gz"
      sha256 "73b0b93fc51e4350cde088aa38f79001343bc428b6dc80480161459c09f030a4"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.7/sin-code-linux-amd64.tar.gz"
      sha256 "2ddbaa3dd2c02e916bc0c9e02ab17bc2e095fa0c607d949d3f0fd298de76a140"
    end
  end

  def install
    bin.install "sin-code"
  end

  test do
    system "#{bin}/sin-code", "--version"
  end
end
