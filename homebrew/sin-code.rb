class SinCode < Formula
  desc "SIN-Code unified toolchain — 13 analysis and manipulation tools in one binary"
  homepage "https://github.com/OpenSIN-Code/SIN-Code-Bundle"
  version "1.0.5"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.5-sin-code/sin-code-darwin-arm64.tar.gz"
      sha256 "c5c0ffeaddc66f25ed99547f20cde11fcc736184551e1d06b8e97fabf6ac4e91"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.5-sin-code/sin-code-darwin-amd64.tar.gz"
      sha256 "dc0b0e3a286df97b0ed9e4f88ed3a0eb543d883f0bf5c51a8eaec0032ec7edda"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.5-sin-code/sin-code-linux-arm64.tar.gz"
      sha256 "e703370c48543744ea9bf293ceefa7e6603e30d0055f1366f57081787f84cb13"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.5-sin-code/sin-code-linux-amd64.tar.gz"
      sha256 "e0adaee52c2a61ff88191c217a90b3e7f3e43662c04fabd7f3d9b350c34f21f0"
    end
  end

  def install
    bin.install "sin-code"
  end

  test do
    system "#{bin}/sin-code", "--version"
  end
end
