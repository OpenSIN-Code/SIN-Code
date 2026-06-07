class SinCode < Formula
  desc "SIN-Code unified toolchain — 13 analysis and manipulation tools in one binary"
  homepage "https://github.com/OpenSIN-Code/SIN-Code-Bundle"
  version "1.0.3"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.3-sin-code/sin-code-darwin-arm64.tar.gz"
      sha256 "d43c5f0cbe797eae9b498558e505f1964d8e4b98712015beb0906e1cd9f315ec"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.3-sin-code/sin-code-darwin-amd64.tar.gz"
      sha256 "67390989de1129824cd4c949d49dfccaaebacb630805f7dcfef010fd39f9e00d"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.3-sin-code/sin-code-linux-arm64.tar.gz"
      sha256 "c03ca8fd41c9037887d2d08edaf87aced46ae631cd26e0bd3e3f30a94651496c"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.3-sin-code/sin-code-linux-amd64.tar.gz"
      sha256 "a381ccecab6f8f101aad820d8b8b49a3f88e2f270555667952523074b30978a5"
    end
  end

  def install
    bin.install "sin-code"
  end

  test do
    system "#{bin}/sin-code", "--version"
  end
end
