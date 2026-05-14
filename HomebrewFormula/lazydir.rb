class Lazydir < Formula
  desc "Terminal UI for browsing AGNTCY Directory instances"
  homepage "https://github.com/agntcy/lazydir"
  version "v0.0.1"
  license "Apache-2.0"
  version_scheme 1

  url "https://github.com/agntcy/lazydir/releases/download/#{version}"

  on_macos do
      if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-darwin-arm64"
          sha256 "1854b4dd5869aa96becac05e7895756352f7f9b80126570783855164d4f76fdd"

          def install
              bin.install "lazydir-darwin-arm64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end

      if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-darwin-amd64"
          sha256 "dd3a4e8621464012b1f137a3a8ced6db04c94e6a4efa08f68fe548249ad1447a"

          def install
              bin.install "lazydir-darwin-amd64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end
  end

  on_linux do
      if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-linux-arm64"
          sha256 "ab59459b2f78b75c249438ca017d6bfc2bd60a24b061399a98a5aed029f0e5b1"

          def install
              bin.install "lazydir-linux-arm64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end

      if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-linux-amd64"
          sha256 "37d63e25eb69c65d0c517945e73bc0da0f56f203d33b31b582bdbc22d7a18c51"

          def install
              bin.install "lazydir-linux-amd64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end
  end
end
