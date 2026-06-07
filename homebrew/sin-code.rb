class SinCode < Formula
  desc "SIN-Code unified toolchain — 13 analysis and manipulation tools in one binary"
  homepage "https://github.com/OpenSIN-Code/SIN-Code-Bundle"
  version "1.0.2"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-darwin-arm64.tar.gz"
      sha256 "f8f7a8062b24be580a567f1dfd64cd8177649b9a7a10c62002aa03daddcbce87"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-darwin-amd64.tar.gz"
      sha256 "7a31752a399617e515835b6ee683b444d8de16f2c3710e17bb9c651fb9a87c5e"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-linux-arm64.tar.gz"
      sha256 "27f2dc3f6f4d2fadc49451eae81fb8933ce1c2b4061122b7a684fc971ec13f27"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-linux-amd64.tar.gz"
      sha256 "0e69e824fb7a59cd8a62b952fd1928df7f15d2ceae9214869fed3ec132a60439"
    end
  end

  def install
    bin.install "sin-code"
  end

  test do
    system "#{bin}/sin-code", "--version"
  end
end
