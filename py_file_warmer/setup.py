from setuptools import setup, find_packages

setup(
    name="file_warmer",
    version="0.0.5",
    description="Library to read file blocks as fast as possible",
    long_description=open("README.md").read(),
    long_description_content_type="text/markdown",
    packages=find_packages(),
    package_data={
        "file_warmer": ["lib/*.h", "lib/*.so"],
    },
    classifiers=[
        "Programming Language :: Python :: 3",
        "License :: OSI Approved :: MIT License",
        "Operating System :: POSIX :: Linux",
    ],
    python_requires=">=3.6",
)
