class SinCode < Formula
  desc "SIN-Code unified toolchain — 46+ analysis and manipulation tools in one binary"
  homepage "https://github.com/OpenSIN-Code/SIN-Code"
  version "3.23.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code/releases/download/v3.23.0/sin-code-darwin-arm64.tar.gz"
      sha256 "c8eaa2064fa3ec39023d630beeb7c7e01eba1e402d72c91f98aedf5983bdcf53"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code/releases/download/v3.23.0/sin-code-darwin-amd64.tar.gz"
      sha256 "58f5c48fb5fb66938859322644cc860071d23784fb7c0fdcf782faf52b68e9ab"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code/releases/download/v3.23.0/sin-code-linux-arm64.tar.gz"
      sha256 "9da5fc847628aeccc6b7ea9bf1fce7f18ce8cb41b6d3194bb5418932d61567d6"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code/releases/download/v3.23.0/sin-code-linux-amd64.tar.gz"
      sha256 "d6e9ee93746624de349a8ceb3e40c676e6a2b114c944a44d1c8df3eeb338df93"
    end
  end

  def install
    bin.install "sin-code"
  end

  test do
    system "#{bin}/sin-code", "--version"
  end
end
